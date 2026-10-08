package cadres

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rafi-ramdhani/cadre/internal/fsx"
	"github.com/rafi-ramdhani/cadre/internal/paths"
)

// OldConfig is where 0.1.x and the bash 0.2.0 work kept machine settings.
func OldConfig() string { return filepath.Join(paths.Home(), ".config", "cadre") }

// CopyOldConfig moves ~/.config/cadre into ~/.cadre/config once (N.1):
//   - home (the default cadre's path) becomes default, by name;
//   - every cadre it names that is not under ~/.cadre (home and the known
//     cadres list) is listed in external, so it keeps working where it is;
//   - the settings fingerprints are kept, merged with any already there.
//
// Then ~/.config/cadre is removed. It does nothing when the old folder is
// not there. It returns what it did, for a one-line note.
func CopyOldConfig() (string, error) {
	old := OldConfig()
	if _, err := os.Stat(old); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	l, err := lockConfig()
	if err != nil {
		return "", err
	}
	defer l.Release()
	if _, err := os.Stat(old); errors.Is(err, fs.ErrNotExist) {
		return "", nil // another command copied it while this one waited
	}
	var outside []string
	add := func(p string) {
		p = paths.Real(expandHome(p))
		if !strings.HasPrefix(p, "/") || Inside(p) {
			return
		}
		for _, x := range outside {
			if x == p {
				return
			}
		}
		outside = append(outside, p)
	}
	home := firstLine(filepath.Join(old, "home"))
	if home != "" {
		add(home)
	}
	if raw, err := os.ReadFile(filepath.Join(old, "cadres")); err == nil {
		for _, l := range strings.Split(string(raw), "\n") {
			if l = strings.TrimSpace(l); strings.HasPrefix(l, "/") {
				add(l)
			}
		}
	}
	list := externalPaths()
	for _, p := range outside {
		known := false
		for _, x := range list {
			known = known || x == p
		}
		if !known {
			list = append(list, p)
		}
	}
	if err := writeExternal(list); err != nil {
		return "", err
	}
	if home != "" && Default() == "" {
		if err := fsx.WriteFile(Config("default"), []byte(filepath.Base(paths.Real(expandHome(home)))+"\n"), 0o600); err != nil {
			return "", err
		}
	}
	if raw, err := os.ReadFile(filepath.Join(old, "persona-settings.sha256")); err == nil {
		merged := string(raw)
		if mine, err := os.ReadFile(Config("persona-settings.sha256")); err == nil {
			merged = mergeHashes(string(raw), string(mine))
		}
		if err := fsx.WriteFile(Config("persona-settings.sha256"), []byte(merged), 0o600); err != nil {
			return "", err
		}
	}
	if err := os.RemoveAll(old); err != nil {
		return "", err
	}
	return "moved cadre's settings from " + old + " to " + ConfigDir(), nil
}

// mergeHashes joins two fingerprint files; lines in newer win by path.
func mergeHashes(older, newer string) string {
	byPath := map[string]string{}
	var order []string
	for _, text := range []string{older, newer} {
		for _, line := range strings.Split(text, "\n") {
			_, p, ok := strings.Cut(line, " ")
			if !ok || p == "" {
				continue
			}
			if _, seen := byPath[p]; !seen {
				order = append(order, p)
			}
			byPath[p] = line
		}
	}
	var b strings.Builder
	for _, p := range order {
		b.WriteString(byPath[p] + "\n")
	}
	return b.String()
}

func firstLine(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(raw), "\n")
	return strings.TrimSpace(line)
}

// expandHome turns a leading ~ into the home folder.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return paths.Home() + p[1:]
	}
	return p
}
