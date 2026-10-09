package cadreis

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rafi-ramdhani/cadrei/internal/fsx"
	"github.com/rafi-ramdhani/cadrei/internal/paths"
	"github.com/rafi-ramdhani/cadrei/internal/registry"
)

// ProjectsDir is the folder new clones go to (config/projects-dir), with ~
// expanded, or "" when it was never set.
func ProjectsDir() string { return expandHome(firstLine(Config("projects-dir"))) }

// Registry is a cadrei's projects.yaml.
func (c Cadrei) Registry() string { return filepath.Join(c.Path, "projects.yaml") }

// Resolved is the cadrei a command acts on, and how it was found.
type Resolved struct {
	Cadrei
	From    string // "from CADREI_HOME", "from this folder", "from the project <name>" or "default"
	Project string // the linked project the current folder is in, when found that way
	Default string // the default cadrei's name
}

// ErrNoCadrei is returned when nothing resolves.
var ErrNoCadrei = errors.New("no cadrei here and no default cadrei; run cadrei init <name>, or cadrei use <name>")

// Asker picks one of several cadreis that link the same project; it is nil
// when there is no terminal to ask on.
type Asker func(project string, names []string) (string, error)

// Resolve finds the cadrei for a command run in cwd (N.3), first match
// first:
//  1. CADREI_HOME, used as given (members and the orchestrator have it set);
//  2. cwd inside a cadrei's folder under ~/.cadrei;
//  3. cwd inside a project a cadrei links, the deepest one when projects
//     nest; when several cadreis link it, ask, or refuse without a terminal;
//  4. the default cadrei.
//
// Configuration is only read from ~/.cadrei or CADREI_HOME, never from a
// folder that merely looks like a cadrei.
func Resolve(cwd string, ask Asker) (*Resolved, error) {
	def := Default()
	cwd = paths.Real(cwd)
	if env := os.Getenv("CADREI_HOME"); env != "" {
		p := paths.Real(env)
		c, ok := At(p)
		if !ok {
			// Another spelling of a listed cadrei (letter case) is that cadrei.
			list, _ := List()
			for _, l := range list {
				if l.Present() && paths.Inside(p, l.Path) && paths.Inside(l.Path, p) {
					c, ok = l, true
				}
			}
		}
		if !ok {
			c = Cadrei{Name: filepath.Base(p), Path: p}
		}
		return &Resolved{Cadrei: c, From: "from CADREI_HOME", Default: def}, nil
	}
	list, err := List()
	if err != nil {
		return nil, err
	}
	var best *Cadrei
	for i, c := range list {
		if c.Present() && paths.Inside(cwd, c.Path) && (best == nil || len(c.Path) > len(best.Path)) {
			best = &list[i]
		}
	}
	if best != nil {
		return checked(&Resolved{Cadrei: *best, From: "from this folder", Default: def}, list)
	}
	if r, err := byProject(cwd, list, ask); r != nil || err != nil {
		if r != nil {
			r.Default = def
			return checked(r, list)
		}
		return nil, err
	}
	if def != "" {
		for _, c := range list {
			if c.Name == def && c.Present() {
				return checked(&Resolved{Cadrei: c, From: "default", Default: def}, list)
			}
		}
	}
	return nil, ErrNoCadrei
}

// checked refuses a cadrei whose name another present cadrei also has.
func checked(r *Resolved, list []Cadrei) (*Resolved, error) {
	for _, c := range list {
		if c.Path != r.Path && strings.EqualFold(c.Name, r.Name) && c.Present() {
			return nil, fmt.Errorf("cadreis at %s and %s share the name %s; rename one of the folders", r.Path, c.Path, r.Name)
		}
	}
	return r, nil
}

type link struct {
	cadrei  Cadrei
	project string
	dir     string
}

// byProject finds the cadrei that links the project cwd is in.
func byProject(cwd string, list []Cadrei, ask Asker) (*Resolved, error) {
	var found []link
	deepest := ""
	for _, c := range list {
		if !c.Present() {
			continue
		}
		reg, err := registry.Load(c.Registry())
		if err != nil {
			continue
		}
		for _, e := range reg.Entries() {
			d := ProjectDir(c, e)
			if d == "" {
				continue
			}
			d = paths.Real(d)
			if !paths.Inside(cwd, d) {
				continue
			}
			switch {
			case len(d) > len(deepest):
				deepest, found = d, []link{{c, e.Name, d}}
			case d == deepest:
				found = append(found, link{c, e.Name, d})
			}
		}
	}
	var cadreis []link
	for _, l := range found {
		dup := false
		for _, x := range cadreis {
			dup = dup || x.cadrei.Path == l.cadrei.Path
		}
		if !dup {
			cadreis = append(cadreis, l)
		}
	}
	switch len(cadreis) {
	case 0:
		return nil, nil
	case 1:
		l := cadreis[0]
		return &Resolved{Cadrei: l.cadrei, From: "from the project " + l.project, Project: l.project}, nil
	}
	names := make([]string, len(cadreis))
	for i, l := range cadreis {
		names[i] = l.cadrei.Name
	}
	project := cadreis[0].project
	if ask == nil {
		return nil, fmt.Errorf("%s is linked by %s; run from ~/.cadrei/<name>, or set CADREI_HOME", project, andList(names))
	}
	pick, err := ask(project, names)
	if err != nil {
		return nil, err
	}
	for _, l := range cadreis {
		if l.cadrei.Name == pick {
			return &Resolved{Cadrei: l.cadrei, From: "from the project " + l.project, Project: l.project}, nil
		}
	}
	return nil, fmt.Errorf("%s is not one of %s", pick, andList(names))
}

// andList writes "a", "a and b", "a, b and c".
func andList(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// Linking returns every present cadrei that links the folder dir (by
// physical path), for cadrei ls in a project two cadreis link.
func Linking(dir string) []Cadrei {
	dir = paths.Real(dir)
	list, _ := List()
	var out []Cadrei
	for _, c := range list {
		if !c.Present() {
			continue
		}
		reg, err := registry.Load(c.Registry())
		if err != nil {
			continue
		}
		for _, e := range reg.Entries() {
			if d := ProjectDir(c, e); d != "" && paths.Real(d) == dir {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// SuggestProjectsDir is where new clones go when the user has not said:
// ~/Developer if it exists, else ~/Projects if it exists, else ~/Developer.
func SuggestProjectsDir() string {
	home := paths.Home()
	for _, d := range []string{"Developer", "Projects"} {
		if st, err := os.Stat(filepath.Join(home, d)); err == nil && st.IsDir() {
			return filepath.Join(home, d)
		}
	}
	return filepath.Join(home, "Developer")
}

// SetProjectsDir records where new clones go, as ~/... under the home
// folder. Only future clones are affected.
func SetProjectsDir(dir string) error {
	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		return err
	}
	return fsx.WriteFile(Config("projects-dir"), []byte(Tilde(paths.Real(dir))+"\n"), 0o600)
}

// Tilde writes a physical path as ~/... when it is under the home folder
// (the registry travels with the cadrei's git, and ~ means the same place
// on the user's other machines), and as it is otherwise.
func Tilde(p string) string {
	home := paths.Home()
	if p == home {
		return "~"
	}
	if paths.Within(p, home) {
		return "~" + p[len(home):]
	}
	return p
}
