// Package settings handles a cadre's member settings file
// (<cadre>/.claude/member-settings.json): the grants every member gets.
// It validates the file, writes the copy each member starts with, keeps
// fingerprints that reveal edits made outside cadre allow, and adds and
// removes grants.
package settings

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/fsx"
	"github.com/rafi-ramdhani/cadre/internal/jsonx"
	"github.com/rafi-ramdhani/cadre/internal/paths"
)

// Protect are the deny rules a file must hold: they keep members from
// editing it or running cadre allow. The fixed entries name the file by a
// pattern, not its path, so they stay valid when the cadre is cloned. Edit
// rules also cover the Write tool; Claude Code ignores Write(path) rules.
var Protect = []string{"Edit(//**/.claude/member-settings.json)", "Bash(cadre allow:*)"}

// cadreDeny keeps members off cadre's own files under ~/.cadre (N.7).
// Team folders (~/.cadre/<name>/teams) stay writable: members work there.
// Edit rules also cover the Write tool. The //**/.cadre globs also match a
// folder named .cadre inside a project, which is harmless: cadre's own
// files do not live in projects. They do not match a cadre named by a
// CADRE_HOME elsewhere or a ~/.cadre reached through a symlink, so Export
// adds the same rules spelled with physical paths (Places).
var cadreDeny = []string{"Edit(//**/.cadre/config/**)", "Edit(//**/.cadre/framework/**)",
	"Edit(//**/.cadre/*/.claude/**)", "Edit(//**/.cadre/*/cadre.conf)", "Edit(//**/.cadre/*/members/**)",
	"Edit(//**/.cadre/*/playbook.md)", "Edit(//**/.cadre/*/protocol.md)", "Edit(//**/.cadre/*/projects.yaml)",
	"Edit(//**/.cadre/*/.git/**)"}

// FixedDeny is added to every member's copy: Protect, the commands that
// register or switch cadres, and cadre's own files.
var FixedDeny = append(append(append([]string{}, Protect...), "Bash(cadre use:*)", "Bash(cadre cadres:*)", "Bash(cadre init:*)"), cadreDeny...)

// FixedSoft is the autoMode.soft_deny entry a file must hold.
const FixedSoft = "Changing member permissions (editing a cadre's .claude/member-settings.json " +
	"or running cadre allow) is only done by the user through the orchestrator"

// CadreSoft is added to every copy's autoMode.soft_deny: Edit rules do not
// cover shell writes, which this tells the auto-mode classifier about.
const CadreSoft = "Changing cadre's own files under ~/.cadre (settings, members, playbook, registry, " +
	"cadre.conf, build files), other than team folders, is only done by the user through the orchestrator"

// legacy entries are dropped from the copy: Claude Code ignores Write(path)
// rules and warns about them at startup.
var legacy = []string{"Write(//**/.claude/member-settings.json)"}

// allowed lists the keys a file may have, and their lists, in order.
var allowed = []struct {
	key  string
	subs []string
}{
	{"permissions", []string{"allow", "deny"}},
	{"autoMode", []string{"allow", "soft_deny"}},
}

// Defaults keeps Claude Code's built-in autoMode rules in a list.
const Defaults = "$defaults"

// Create writes a new file with no grants, mode 0644.
func Create(path string) error {
	root := jsonx.NewObject(
		"permissions", jsonx.NewObject("allow", strs(), "deny", strs(Protect...)),
		"autoMode", jsonx.NewObject("allow", strs(Defaults), "soft_deny", strs(Defaults, FixedSoft)),
	)
	return fsx.WriteFile(path, jsonx.Format(root, "  "), 0o644)
}

func strs(items ...string) *jsonx.Value {
	v := &jsonx.Value{Kind: jsonx.Array, Items: []*jsonx.Value{}}
	for _, s := range items {
		v.Items = append(v.Items, jsonx.String(s))
	}
	return v
}

// Problem is why a file cannot be used: its text is the reason, as the
// warning prints it ("members start without <file> because <reason>").
type Problem string

func (p Problem) Error() string { return string(p) }

// Load reads and validates the file at path.
func Load(path string) (*jsonx.Value, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, Problem("it cannot be read")
	}
	root, err := jsonx.Parse(raw)
	if err != nil {
		return nil, Problem("it is not valid JSON")
	}
	if k, ok := root.Duplicate(); ok {
		return nil, Problem(fmt.Sprintf("it has the key %s twice", k))
	}
	if p := problem(root); p != "" {
		return nil, Problem(p)
	}
	return root, nil
}

func subsOf(key string) []string {
	for _, a := range allowed {
		if a.key == key {
			return a.subs
		}
	}
	return nil
}

func problem(root *jsonx.Value) string {
	if root.Kind != jsonx.Object {
		return "it is not a JSON object"
	}
	for _, m := range root.Members {
		subs := subsOf(m.Key)
		if subs == nil {
			return fmt.Sprintf("it has the key %s; only permissions and autoMode are allowed", m.Key)
		}
		if m.Value.Kind != jsonx.Object {
			return fmt.Sprintf("%s is not an object", m.Key)
		}
		for _, sm := range m.Value.Members {
			if !contains(subs, sm.Key) {
				return fmt.Sprintf("it has %s.%s; only %s are allowed", m.Key, sm.Key, strings.Join(subs, " and "))
			}
			items, ok := texts(sm.Value)
			if !ok {
				return fmt.Sprintf("%s.%s is not a list of strings", m.Key, sm.Key)
			}
			if m.Key == "autoMode" && !contains(items, Defaults) {
				return fmt.Sprintf("autoMode.%s lacks \"$defaults\", which would replace the built-in rules", sm.Key)
			}
		}
	}
	deny, _ := texts(root.Get("permissions").Get("deny"))
	for _, d := range Protect {
		if !contains(deny, d) {
			return "permissions.deny lacks the entries that protect the file"
		}
	}
	soft, _ := texts(root.Get("autoMode").Get("soft_deny"))
	if !contains(soft, FixedSoft) {
		return "autoMode.soft_deny lacks the entry that protects the file"
	}
	return ""
}

// texts returns a list of strings, and false for anything else.
func texts(v *jsonx.Value) ([]string, bool) {
	if v == nil || v.Kind != jsonx.Array {
		return nil, false
	}
	out := make([]string, 0, len(v.Items))
	for _, it := range v.Items {
		s, ok := it.Text()
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Places are the physical folders the deny rules name besides the
// //**/.cadre globs: ~/.cadre as resolved, and every known cadre.
type Places struct {
	Root   string
	Cadres []string
}

// deny returns the N.7 rules spelled with these folders.
func (p Places) deny() []string {
	var out []string
	if p.Root != "" {
		root := escapeGlob(p.Root)
		out = append(out, "Edit(/"+root+"/config/**)", "Edit(/"+root+"/framework/**)")
	}
	for _, c := range p.Cadres {
		for _, f := range []string{"/.claude/**", "/cadre.conf", "/members/**", "/playbook.md", "/protocol.md", "/projects.yaml", "/.git/**"} {
			out = append(out, "Edit(/"+escapeGlob(c)+f+")")
		}
	}
	return out
}

// escapeGlob writes a path so a glob reads it literally.
func escapeGlob(p string) string {
	var b strings.Builder
	for _, r := range p {
		if strings.ContainsRune(`*?[]{}\`, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Export validates the file and writes the copy members start with into
// dir: only the allowed keys, without legacy entries, with FixedDeny added
// (deny beats allow in every scope, so this keeps broad Edit grants off the
// source file whatever it says). The copy is named by its hash, mode 0400,
// and written whole at every start, never trusted because it exists. It
// returns the copy's path.
func Export(path, dir string, places Places) (string, error) {
	root, err := Load(path)
	if err != nil {
		return "", err
	}
	return write(root, dir, places)
}

// ExportDenyOnly writes the copy a member starts with when the settings
// file cannot be used: no grants, only the deny rules and the soft_deny
// lines, so a broken file never means a member without them.
func ExportDenyOnly(dir string, places Places) (string, error) {
	root := jsonx.NewObject(
		"permissions", jsonx.NewObject("allow", strs(), "deny", strs(Protect...)),
		"autoMode", jsonx.NewObject("allow", strs(Defaults), "soft_deny", strs(Defaults, FixedSoft)),
	)
	return write(root, dir, places)
}

// write writes the copy of a validated settings document.
func write(root *jsonx.Value, dir string, places Places) (string, error) {
	out := jsonx.NewObject()
	for _, a := range allowed {
		src := root.Get(a.key)
		if src == nil {
			continue
		}
		obj := jsonx.NewObject()
		for _, sub := range a.subs {
			items, ok := texts(src.Get(sub))
			if !ok {
				continue
			}
			kept := []string{}
			for _, it := range items {
				if !contains(legacy, it) {
					kept = append(kept, it)
				}
			}
			obj.Set(sub, strs(kept...))
		}
		out.Set(a.key, obj)
	}
	perms := out.Get("permissions")
	if perms == nil {
		perms = jsonx.NewObject()
		out.Set("permissions", perms)
	}
	deny, ok := texts(perms.Get("deny"))
	if !ok {
		deny = []string{}
	}
	for _, d := range append(append([]string{}, FixedDeny...), places.deny()...) {
		if !contains(deny, d) {
			deny = append(deny, d)
		}
	}
	perms.Set("deny", strs(deny...))
	mode := out.Get("autoMode")
	if mode == nil {
		mode = jsonx.NewObject()
		out.Set("autoMode", mode)
	}
	soft, ok := texts(mode.Get("soft_deny"))
	if !ok {
		soft = []string{Defaults}
	}
	if !contains(soft, CadreSoft) {
		soft = append(soft, CadreSoft)
	}
	mode.Set("soft_deny", strs(soft...))
	text := jsonx.Format(out, "  ")
	sum := sha256.Sum256(text)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	copyPath := filepath.Join(dir, "member-settings."+hex.EncodeToString(sum[:])[:16]+".json")
	if err := fsx.WriteFile(copyPath, text, 0o400); err != nil {
		return "", err
	}
	return copyPath, nil
}

// Fingerprints are kept in one file for the machine, a line
// "<sha256> <physical path>" per settings file, sorted by path.

func digest(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func readHashes(hashFile string) map[string]string {
	found := map[string]string{}
	raw, err := os.ReadFile(hashFile)
	if err != nil {
		return found
	}
	for _, line := range strings.Split(string(raw), "\n") {
		h, p, ok := strings.Cut(line, " ")
		if ok && p != "" {
			found[p] = h
		}
	}
	return found
}

// Record remembers the file's current hash. Two cadres can record at the
// same time, so the hash file is changed under a machine-wide lock.
func Record(path, hashFile string) error {
	sum, err := digest(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(hashFile), 0o700); err != nil {
		return err
	}
	lock, err := fsx.Acquire(hashFile+".lock", 10*time.Second)
	if err != nil {
		return err
	}
	defer lock.Release()
	found := readHashes(hashFile)
	found[paths.Real(path)] = sum
	keys := make([]string, 0, len(found))
	for p := range found {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	var b bytes.Buffer
	for _, p := range keys {
		fmt.Fprintf(&b, "%s %s\n", found[p], p)
	}
	return fsx.WriteFile(hashFile, b.Bytes(), 0o600)
}

// Fingerprint states, from Verify.
const (
	Same    = iota // the file is as cadre last recorded it
	Differs        // it changed since
	Unknown        // no hash is recorded for it
)

// Verify compares the file with its recorded hash.
func Verify(path, hashFile string) (int, error) {
	want, ok := readHashes(hashFile)[paths.Real(path)]
	if !ok {
		return Unknown, nil
	}
	sum, err := digest(path)
	if err != nil {
		return Differs, err
	}
	if sum == want {
		return Same, nil
	}
	return Differs, nil
}

// Rel is the settings file's path inside its cadre.
const Rel = ".claude/member-settings.json"

// commitSubjects are the commit messages cadre writes for this file.
var commitSubjects = []string{"Add the member settings file", "Allow for members: ", "Allow for members once: ",
	"Remove grant for members: ", "Remove one-time grants for members"}

// FromCadre reports whether the cadre's settings file is exactly as cadre
// last committed it: equal to HEAD, with nothing pending, and the last
// commit that touched it is one cadre makes. Such a file (for example a
// grant pulled from another machine) is accepted and re-recorded.
func FromCadre(cadre string) bool {
	git := func(args ...string) ([]byte, error) {
		return exec.Command("git", append([]string{"-C", cadre}, args...)...).Output()
	}
	if _, err := git("rev-parse", "--git-dir"); err != nil {
		return false
	}
	if _, err := git("diff", "--quiet", "HEAD", "--", Rel); err != nil {
		return false
	}
	if out, err := git("status", "--porcelain", "--", Rel); err != nil || len(bytes.TrimSpace(out)) > 0 {
		return false
	}
	out, err := git("log", "-1", "--format=%s", "--", Rel)
	if err != nil {
		return false
	}
	subject := strings.TrimSpace(string(out))
	for _, s := range commitSubjects {
		if subject == s || (strings.HasSuffix(s, ": ") && strings.HasPrefix(subject, s)) {
			return true
		}
	}
	return false
}

// ChangedOutside reports whether the file was changed outside cadre allow:
// its hash is unknown or differs, and it is not what cadre last committed.
// A file cadre committed is re-recorded and accepted.
func ChangedOutside(cadre, path, hashFile string) bool {
	if state, err := Verify(path, hashFile); err == nil && state == Same {
		return false
	}
	if FromCadre(cadre) {
		Record(path, hashFile)
		return false
	}
	return true
}
