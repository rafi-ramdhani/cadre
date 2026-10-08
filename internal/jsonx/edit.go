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
