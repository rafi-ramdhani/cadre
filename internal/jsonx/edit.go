package jsonx

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/rafi-ramdhani/cadre/internal/paths"
)

// The outcome of an Edit. The numbers are the bash json_edit exit codes,
// which messages and tests rely on.
const (
	Changed     = 0 // written (or, for a dry run, would be)
	Unchanged   = 3 // nothing to change; not written, no backup
	Missing     = 4 // the file does not exist; it is never created
	Unusable    = 5 // unreadable, not valid JSON, an unexpected shape, or another user's
	KeptChanged = 6 // the file kept changing while cadre wrote it; left as it was
	WriteFailed = 7 // the new file could not be written; the old one stands
)

// An Op changes a document in place and returns lines that report what it
// did, and whether it changed anything. A *ShapeError leaves the file alone.
type Op func(root *Value) (lines []string, changed bool, err error)

// Options for Edit.
type Options struct {
	Backup string // the backup's path
	// FreshBackup writes the backup on every change. Otherwise it is
	// written only if it does not exist, so the first original is kept.
	FreshBackup bool
	DryRun      bool // run the op and report, but write nothing
}

// attempts is how often Edit tries when the file changes under it.
const attempts = 3

// Edit changes the JSON file at path with op, safely:
//   - the file is never created, and a symlink is followed to the file;
//   - a file that is unreadable, not valid JSON, the wrong shape, or owned
//     by another user is left alone;
//   - before the first change, the original is copied to the backup;
//   - the new file is written next to the old one with the same mode, and
//     renamed over it only if the old file has not changed in the meantime
//     (size, mtime and SHA-256); otherwise it tries again, then gives up;
//   - keys, their order, and every value op does not touch are written back
//     byte for byte, with the indentation the file had, and a final newline
//     only when it had one.
//
// raceHook, when set (by tests, see hook*.go), runs between writing the new
// file and the check, so a test can change the file at that moment.
func Edit(path string, opts Options, op Op) (int, []string) {
	path = paths.Real(path)
	for n := 1; n <= attempts; n++ {
		code, lines, retry := attempt(n, path, opts, op)
		if !retry {
			return code, lines
		}
	}
	return KeptChanged, nil
}

type stamp struct {
	size  int64
	mtime int64
	sum   [32]byte
}

func snapshot(path string) (stamp, []byte, os.FileInfo, error) {
	st, err := os.Stat(path)
	if err != nil {
		return stamp{}, nil, nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return stamp{}, nil, nil, err
	}
	return stamp{st.Size(), st.ModTime().UnixNano(), sha256.Sum256(raw)}, raw, st, nil
}

func attempt(n int, path string, opts Options, op Op) (code int, lines []string, retry bool) {
	before, raw, st, err := snapshot(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Missing, nil, false
	}
	if err != nil || !st.Mode().IsRegular() {
		return Unusable, nil, false
	}
	root, err := Parse(raw)
	if err != nil {
		return Unusable, nil, false
	}
	lines, changed, err := op(root)
	if err != nil {
		return Unusable, nil, false
	}
	if !changed {
		return Unchanged, lines, false
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok && int(sys.Uid) != os.Geteuid() {
		return Unusable, nil, false
	}
	if opts.DryRun {
		return Changed, lines, false
	}
	mode := st.Mode().Perm()
	if opts.Backup != "" {
		if _, err := os.Lstat(opts.Backup); opts.FreshBackup || errors.Is(err, fs.ErrNotExist) {
			if err := writeTemp(opts.Backup, raw, mode); err != nil {
				return WriteFailed, nil, false
			}
		}
	}
	out := bytes.TrimRight(Format(root, Indent(raw)), "\n")
	if bytes.HasSuffix(raw, []byte("\n")) {
		out = append(out, '\n')
	}
	tmp, err := tempNext(path, out, mode)
	if err != nil {
		return WriteFailed, nil, false
	}
	defer os.Remove(tmp) // gone after a rename; removed after a failure
	if raceHook != nil {
		raceHook(n, path)
	}
	now, _, _, err := snapshot(path)
	if err != nil || now != before {
		return 0, nil, true
	}
	if err := os.Rename(tmp, path); err != nil {
		return WriteFailed, nil, false
	}
	return Changed, lines, false
}

// tempNext writes data to a new file in path's folder with mode and
// returns its name.
func tempNext(path string, data []byte, mode fs.FileMode) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".cadre-")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err := f.Chmod(mode); err == nil {
		if _, err = f.Write(data); err == nil {
			err = f.Sync()
		}
		if cerr := f.Close(); err == nil && cerr == nil {
			return name, nil
		}
	} else {
		f.Close()
	}
	os.Remove(name)
	return "", errors.New("could not write " + name)
}

// writeTemp writes data to path through a temporary file and a rename.
func writeTemp(path string, data []byte, mode fs.FileMode) error {
	tmp, err := tempNext(path, data, mode)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// TrustEntry names a project and the folder keys to trust it under (its
// physical path and, when different, the path as cadre spells it).
type TrustEntry struct {
	Name string
	Dirs []string
}

// Trust marks folders as trusted in Claude Code's config
// (projects[<dir>].hasTrustDialogAccepted = true). It reports
// "<name>\ttrusted" or "<name>\talready" for each entry.
func Trust(entries []TrustEntry) Op {
	return func(root *Value) ([]string, bool, error) {
		projects, err := projectsOf(root, true)
		if err != nil {
			return nil, false, err
		}
		var lines []string
		changed := false
		for _, e := range entries {
			all := true
			for _, d := range e.Dirs {
				if !projects.Get(d).Get("hasTrustDialogAccepted").IsTrue() {
					all = false
				}
			}
			if all {
				lines = append(lines, e.Name+"\talready")
				continue
			}
			for _, d := range e.Dirs {
				entry := projects.Get(d)
				if entry == nil {
					entry = NewObject()
					projects.Set(d, entry)
				}
				if entry.Kind != Object {
					return nil, false, &ShapeError{"projects entry is not an object"}
				}
				entry.Set("hasTrustDialogAccepted", Literal("true"))
			}
			lines = append(lines, e.Name+"\ttrusted")
			changed = true
		}
		return lines, changed, nil
	}
}

// Untrust removes hasTrustDialogAccepted from the folders' entries and
// leaves every other key (section L). It reports "<name>\tuntrusted" or
// "<name>\tnot trusted".
func Untrust(entries []TrustEntry) Op {
	return func(root *Value) ([]string, bool, error) {
		projects, err := projectsOf(root, false)
		if err != nil {
			return nil, false, err
		}
		var lines []string
		changed := false
		for _, e := range entries {
			removed := false
			for _, d := range e.Dirs {
				entry := projects.Get(d)
				if entry == nil {
					continue
				}
				if entry.Kind != Object {
					return nil, false, &ShapeError{"projects entry is not an object"}
				}
				if entry.Delete("hasTrustDialogAccepted") {
					removed = true
				}
			}
			if removed {
				lines = append(lines, e.Name+"\tuntrusted")
				changed = true
			} else {
				lines = append(lines, e.Name+"\tnot trusted")
			}
		}
		return lines, changed, nil
	}
}

// projectsOf returns root's projects object, made when create is set.
func projectsOf(root *Value, create bool) (*Value, error) {
	if root.Kind != Object {
		return nil, &ShapeError{"the top level is not an object"}
	}
	projects := root.Get("projects")
	if projects == nil {
		projects = NewObject()
		if create {
			root.Set("projects", projects)
		}
	}
	if projects.Kind != Object {
		return nil, &ShapeError{"projects is not an object"}
	}
	return projects, nil
}

// sessionStart returns the SessionStart groups of a settings file, made
// when create is set, after checking the shape on the way.
func sessionStart(root *Value, create bool) (hooks, groups *Value, err error) {
	if root.Kind != Object {
		return nil, nil, &ShapeError{"the top level is not an object"}
	}
	hooks = root.Get("hooks")
	if hooks == nil {
		if !create {
			return nil, nil, nil
		}
		hooks = NewObject()
		root.Set("hooks", hooks)
	}
	if hooks.Kind != Object {
		return nil, nil, &ShapeError{"hooks is not an object"}
	}
	groups = hooks.Get("SessionStart")
	if groups == nil {
		if !create {
			return hooks, nil, nil
		}
		groups = &Value{Kind: Array, Items: []*Value{}}
		hooks.Set("SessionStart", groups)
	}
	if groups.Kind != Array {
		return nil, nil, &ShapeError{"hooks.SessionStart is not a list"}
	}
	for _, g := range groups.Items {
		if g.Kind != Object || g.Get("hooks") == nil || g.Get("hooks").Kind != Array {
			return nil, nil, &ShapeError{"a SessionStart group has no hooks list"}
		}
	}
	return hooks, groups, nil
}

// commandOf returns a hook's command, if it has one.
func commandOf(h *Value) (string, bool) {
	if h.Kind != Object {
		return "", false
	}
	return h.Get("command").Text()
}

// AddHook makes command the SessionStart hook for which isCadre is true:
// groups holding any such hook are dropped and one group with command is
// added. Unchanged when command is already the only one.
func AddHook(command string, isCadre func(command string) bool) Op {
	return func(root *Value) ([]string, bool, error) {
		_, groups, err := sessionStart(root, true)
		if err != nil {
			return nil, false, err
		}
		var found []string
		for _, g := range groups.Items {
			for _, h := range g.Get("hooks").Items {
				if c, ok := commandOf(h); ok && isCadre(c) {
					found = append(found, c)
				}
			}
		}
		if len(found) == 1 && found[0] == command {
			return []string{"already"}, false, nil
		}
		kept := []*Value{}
		for _, g := range groups.Items {
			ours := false
			for _, h := range g.Get("hooks").Items {
				if c, ok := commandOf(h); ok && isCadre(c) {
					ours = true
				}
			}
			if !ours {
				kept = append(kept, g)
			}
		}
		hook := NewObject("type", String("command"), "command", String(command), "timeout", Literal("10"))
		group := NewObject("hooks", &Value{Kind: Array, Items: []*Value{hook}})
		groups.Items = append(kept, group)
		return []string{"added"}, true, nil
	}
}

// Unhook removes the SessionStart hooks for which isOurs is true, and
// groups left empty by that. It reports "removed\t<command>" for each, and
// "kept\t<command>" for hooks isCadre recognises as another cadre
// framework's.
func Unhook(isOurs, isCadre func(command string) bool) Op {
	return func(root *Value) ([]string, bool, error) {
		hooks, groups, err := sessionStart(root, false)
		if err != nil || groups == nil {
			return nil, false, err
		}
		var removed, others []string
		kept := []*Value{}
		for _, g := range groups.Items {
			list := g.Get("hooks")
			rest := []*Value{}
			for _, h := range list.Items {
				c, ok := commandOf(h)
				switch {
				case ok && isOurs(c):
					removed = append(removed, c)
				case ok && isCadre(c):
					others = append(others, c)
					rest = append(rest, h)
				default:
					rest = append(rest, h)
				}
			}
			if len(rest) == len(list.Items) {
				kept = append(kept, g)
			} else if len(rest) > 0 {
				list.Items = rest
				kept = append(kept, g)
			}
		}
		var lines []string
		for _, c := range removed {
			lines = append(lines, "removed\t"+c)
		}
		for _, c := range others {
			lines = append(lines, "kept\t"+c)
		}
		if len(removed) == 0 {
			return lines, false, nil
		}
		if len(kept) > 0 {
			groups.Items = kept
		} else {
			hooks.Delete("SessionStart")
			if len(hooks.Members) == 0 {
				root.Delete("hooks")
			}
		}
		return lines, true, nil
	}
}

// Equal reports whether two documents have the same structure and the same
// bytes for every key and scalar.
func Equal(a, b *Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case Scalar:
		return bytes.Equal(a.Raw, b.Raw)
	case Object:
		if len(a.Members) != len(b.Members) {
			return false
		}
		for i := range a.Members {
			if !bytes.Equal(a.Members[i].RawKey, b.Members[i].RawKey) || !Equal(a.Members[i].Value, b.Members[i].Value) {
				return false
			}
		}
	case Array:
		if len(a.Items) != len(b.Items) {
			return false
		}
		for i := range a.Items {
			if !Equal(a.Items[i], b.Items[i]) {
				return false
			}
		}
	}
	return true
}
