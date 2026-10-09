// Package cadres knows where cadres live: each in its own folder under
// ~/.cadre. It lists them, keeps the default, resolves the cadre a command
// acts on, and creates new ones.
package cadres

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/fsx"
	"github.com/rafi-ramdhani/cadre/internal/paths"
)

// Root is ~/.cadre.
func Root() string { return filepath.Join(paths.Home(), ".cadre") }

// ConfigDir is ~/.cadre/config: machine settings, in no cadre's git.
func ConfigDir() string { return filepath.Join(Root(), "config") }

// Config is a file in ConfigDir.
func Config(name string) string { return filepath.Join(ConfigDir(), name) }

// Cadre is one cadre: a folder ~/.cadre/<name> with members/.
type Cadre struct {
	Name string // the folder's name
	Path string // physical
}

// Present reports whether the cadre's folder is there.
func (c Cadre) Present() bool {
	st, err := os.Stat(filepath.Join(c.Path, "members"))
	return err == nil && st.IsDir()
}

var nameRule = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// reserved names cannot be cadres: they are ~/.cadre's own folders.
var reserved = map[string]bool{"config": true, "framework": true}

// CheckName refuses a name that cannot be a cadre's: it is part of every
// session name, so it may use letters, digits, - and _, and config and
// framework are ~/.cadre's own folders.
func CheckName(name string) error {
	if !nameRule.MatchString(name) {
		return fmt.Errorf("a cadre's name may use letters, digits, - and _ (got %s)", name)
	}
	if reserved[strings.ToLower(name)] {
		return fmt.Errorf("%s is reserved for cadre's own files; choose another name", name)
	}
	return nil
}

// List returns every cadre: the folders under ~/.cadre that hold
// members/, sorted by name. Cadres live only there.
func List() ([]Cadre, error) {
	var out []Cadre
	entries, err := os.ReadDir(Root())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		n := e.Name()
		// A symlink here would make any folder a cadre without cadres add,
		// and a name that cannot be a cadre's would break session names.
		if e.Type()&fs.ModeSymlink != 0 || CheckName(n) != nil {
			continue
		}
		c := Cadre{Name: n, Path: paths.Real(filepath.Join(Root(), n))}
		if c.Present() {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Find returns the cadre called name.
func Find(name string) (Cadre, bool) {
	list, _ := List()
	for _, c := range list {
		if c.Name == name {
			return c, true
		}
	}
	return Cadre{}, false
}

// At returns the listed cadre whose folder is path (physical).
func At(path string) (Cadre, bool) {
	list, _ := List()
	for _, c := range list {
		if c.Path == path {
			return c, true
		}
	}
	return Cadre{}, false
}

// Clash returns a present cadre, other than at path, whose name equals
// name in any letter case: names are addresses, so two cannot share one.
func Clash(name, path string) (Cadre, bool) {
	list, _ := List()
	for _, c := range list {
		if strings.EqualFold(c.Name, name) && c.Path != path && c.Present() {
			return c, true
		}
	}
	return Cadre{}, false
}

// Default returns the default cadre's name, or "".
func Default() string {
	raw, err := os.ReadFile(Config("default"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// SetDefault makes the cadre called name the default.
func SetDefault(name string) error {
	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		return err
	}
	return fsx.WriteFile(Config("default"), []byte(name+"\n"), 0o600)
}

// lockConfig serializes changes to the files in config/.
func lockConfig() (*fsx.Lock, error) {
	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		return nil, err
	}
	l, err := fsx.Acquire(Config(".lock"), 10*time.Second)
	if errors.Is(err, fsx.ErrBusy) {
		return nil, fmt.Errorf("another cadre command is changing %s; try again", ConfigDir())
	}
	return l, err
}

// Inside reports whether path is inside ~/.cadre (physical paths).
func Inside(path string) bool { return paths.Within(path, paths.Real(Root())) }
