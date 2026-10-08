package project

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/registry"
)

var nameRule = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// CheckName refuses a project name that could leave its folder.
func CheckName(name string) error {
	if !nameRule.MatchString(name) || strings.Contains(name, "..") {
		return errors.New("project name may use letters, digits, ., - and _ (no ..)")
	}
	return nil
}

// Clone clones repo (owner/repo through gh when signed in, else over
// https; a URL or local path as it is) into dir. A repo comes from a
// registry that may be shared, so one that starts with - (an option to
// git) is refused, and every word after the options is passed after --.
func Clone(repo, dir string) error {
	if strings.HasPrefix(repo, "-") || strings.HasPrefix(dir, "-") {
		return fmt.Errorf("could not clone %s: a repo cannot start with -", repo)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch {
	case strings.Count(repo, "/") >= 2 || strings.HasPrefix(repo, "/") || strings.HasPrefix(repo, ".") || strings.Contains(repo, ":"):
		cmd = exec.Command("git", "clone", "-q", "--", repo, dir)
	case ghSignedIn():
		cmd = exec.Command("gh", "repo", "clone", repo, dir, "--", "-q")
	default:
		cmd = exec.Command("git", "clone", "-q", "--", "https://github.com/"+repo+".git", dir)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not clone %s: %s", repo, strings.TrimSpace(string(out)))
	}
	return nil
}

func ghSignedIn() bool {
	if _, err := exec.LookPath("gh"); err != nil {
		return false
	}
	return exec.Command("gh", "auth", "status").Run() == nil
}

// SameRepo reports whether two spellings name one repository: the same
// owner/repo, and the same host when both name one (owner/repo alone names
// none).
func SameRepo(a, b string) bool {
	if OwnerRepo(a) != OwnerRepo(b) || OwnerRepo(a) == "" {
		return false
	}
	ha, hb := host(a), host(b)
	return ha == "" || hb == "" || ha == hb
}

// host is the host a repository URL names, lower case, or "".
func host(repo string) string {
	r := strings.TrimSpace(repo)
	switch {
	case strings.Contains(r, "://"):
		r = r[strings.Index(r, "://")+3:]
		if at := strings.Index(r, "@"); at >= 0 && at < strings.Index(r+"/", "/") {
			r = r[at+1:]
		}
		r, _, _ = strings.Cut(r, "/")
		r, _, _ = strings.Cut(r, ":")
	case strings.Contains(r, ":") && !strings.HasPrefix(r, "/"):
		r, _, _ = strings.Cut(r, ":")
		if at := strings.LastIndex(r, "@"); at >= 0 {
			r = r[at+1:]
		}
	default:
		return ""
	}
	return strings.ToLower(r)
}

// OwnerRepo reduces a repository spelling (owner/repo, an https or ssh URL,
// with or without .git) to owner/repo, lower case, for comparing.
func OwnerRepo(repo string) string {
	r := strings.TrimSuffix(strings.TrimSpace(repo), "/")
	r = strings.TrimSuffix(r, ".git")
	if i := strings.LastIndexAny(r, ":/"); i >= 0 {
		if j := strings.LastIndexAny(r[:i], ":/"); j >= 0 {
			r = r[j+1:]
		}
	}
	return strings.ToLower(r)
}

// Origin returns a folder's remote.origin.url, or "".
func Origin(dir string) string {
	out, err := exec.Command("git", "-C", dir, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// TopLevel reports whether dir (physical) is the top folder of a git
// repository; a linked worktree counts.
func TopLevel(dir string) bool {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	return err == nil && paths.Real(strings.TrimSpace(string(out))) == dir
}

// Spec is a project to add.
type Spec struct {
	Name, Repo, Team, About string
}

// Added says what Add or Link did.
type Added struct {
	Dir   string // the project's folder
	Where string // "cloned to <dir>", "already at <dir>" or "linked at <dir>"
	Notes []string
}

// Add registers a project and clones it (section N.2): into the projects
// folder for a cadre in ~/.cadre, into <cadre>/projects/ for an outside
// cadre, as in 0.1.x. A folder already there is linked when it is the same
// repository and refused otherwise. The clone comes first, so a failed
// clone registers nothing.
func Add(c cadres.Cadre, s Spec, projectsDir string, protected []string) (Added, error) {
	var a Added
	if err := CheckName(s.Name); err != nil {
		return a, err
	}
	reg, err := registry.Load(c.Registry())
	if err != nil {
		return a, err
	}
	if reg.Get(s.Name) != nil {
		return a, fmt.Errorf("project '%s' is already in projects.yaml", s.Name)
	}
	dir := filepath.Join(c.Path, "projects", s.Name)
	if !c.External {
		dir = filepath.Join(projectsDir, s.Name)
	}
	if why := DestRefusal(paths.Real(dir), protected); why != "" {
		return a, fmt.Errorf("cannot clone into %s: %s", dir, why)
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		if !SameRepo(Origin(dir), s.Repo) {
			return a, fmt.Errorf("%s exists and is not a clone of %s; link it with cadre project add %s --path %s, or choose another name", dir, s.Repo, s.Name, dir)
		}
		a.Where = "already at " + dir
	} else {
		if err := Clone(s.Repo, dir); err != nil {
			return a, err
		}
		a.Where = "cloned to " + dir
	}
	a.Dir = dir
	fields := []registry.Field{{Key: "repo", Value: s.Repo}, {Key: "team", Value: s.Team}, {Key: "about", Value: s.About}}
	if !c.External {
		fields = append(fields, registry.Field{Key: "path", Value: cadres.Tilde(paths.Real(dir))})
	}
	reg.Add(s.Name, fields...)
	if err := reg.Save(c.Registry()); err != nil {
		return a, err
	}
	note, err := cadres.Commit(c.Path, "Add project "+s.Name, "projects.yaml")
	if note != "" {
		a.Notes = append(a.Notes, note)
	}
	return a, err
}

// LinkRefusal says why a folder (physical) cannot be linked as a project,
// or "" (L.7, with N's ~/.cadre).
func LinkRefusal(dir string, protected []string) string {
	if why := DestRefusal(dir, protected); why != "" {
		return why
	}
	if !TopLevel(dir) {
		return "it is not the top folder of a git repository"
	}
	return ""
}

// DestRefusal says why a folder (physical) cannot hold a project, whether
// linked or cloned there, or "": a registry may be shared, so its paths are
// checked like a link. protected lists the folders the runtime loads code
// from (a project there would be loaded as a skill, for example).
func DestRefusal(dir string, protected []string) string {
	home := paths.Home()
	root := paths.Real(cadres.Root())
	switch {
	case dir == "/":
		return "it is the root folder"
	case dir == home:
		return "it is your home folder"
	case paths.Within(dir, root):
		return "it is inside ~/.cadre, where cadre keeps its own files"
	case paths.Within(root, dir):
		return "it contains ~/.cadre"
	}
	for _, p := range protected {
		switch {
		case paths.Within(dir, p):
			return "it is inside " + cadres.Tilde(p)
		case paths.Within(p, dir):
			return "it contains " + cadres.Tilde(p)
		}
	}
	list, _ := cadres.List()
	for _, c := range list {
		if paths.Within(c.Path, dir) {
			return "it contains the cadre " + c.Name
		}
		if paths.Within(dir, c.Path) && !paths.Within(dir, filepath.Join(c.Path, "projects")) {
			return "it is inside the cadre " + c.Name
		}
	}
	return ""
}

// Link registers a folder the user already has (L.7): it must be the top
// folder of a git repository and not cadre's own. The repo is the one given,
// else the folder's origin. The path is stored as ~/... under home.
func Link(c cadres.Cadre, s Spec, dir string, protected []string) (Added, error) {
	var a Added
	if err := CheckName(s.Name); err != nil {
		return a, err
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return a, fmt.Errorf("%s is not a folder", dir)
	}
	phys := paths.Real(dir)
	if why := LinkRefusal(phys, protected); why != "" {
		return a, fmt.Errorf("%s cannot be linked: %s", dir, why)
	}
	reg, err := registry.Load(c.Registry())
	if err != nil {
		return a, err
	}
	if reg.Get(s.Name) != nil {
		return a, fmt.Errorf("project '%s' is already in projects.yaml", s.Name)
	}
	for _, e := range reg.Entries() {
		if d := cadres.ProjectDir(c, e); d != "" && paths.Real(d) == phys {
			return a, fmt.Errorf("%s is already linked as %s", dir, e.Name)
		}
	}
	for _, other := range cadres.Linking(phys) {
		if other.Path != c.Path {
			a.Notes = append(a.Notes, "note: the cadre "+other.Name+" links this folder too")
		}
	}
	repo := s.Repo
	if repo == "" {
		repo = Origin(phys)
	}
	if repo == "" {
		a.Notes = append(a.Notes, "note: it has no repo, so cadre project sync cannot clone it on another machine")
	}
	reg.Add(s.Name, registry.Field{Key: "repo", Value: repo}, registry.Field{Key: "team", Value: s.Team},
		registry.Field{Key: "about", Value: s.About}, registry.Field{Key: "path", Value: cadres.Tilde(phys)})
	if err := reg.Save(c.Registry()); err != nil {
		return a, err
	}
	note, err := cadres.Commit(c.Path, "Add project "+s.Name, "projects.yaml")
	if note != "" {
		a.Notes = append(a.Notes, note)
	}
	a.Dir, a.Where = phys, "linked at "+phys
	return a, err
}

// SyncResult is what Sync did with one project.
type SyncResult struct {
	Name, State, Dir string // State: "present", "cloned", "no repo", "no folder", "failed"
	Err              error
}

// Sync clones the registry's projects missing on this machine to their
// recorded folders (N.2), creating parent folders under the home folder
// only. noDir is returned when a project has no folder because the
// projects folder is not set.
func Sync(c cadres.Cadre, protected []string) ([]SyncResult, error) {
	reg, err := registry.Load(c.Registry())
	if err != nil {
		return nil, err
	}
	var out []SyncResult
	for _, e := range reg.Entries() {
		d := cadres.ProjectDir(c, e)
		r := SyncResult{Name: e.Name, Dir: d}
		switch st, err := os.Stat(d); {
		case d == "":
			r.State = "no folder"
		case err == nil && st.IsDir():
			r.State = "present"
		case e.Get("repo") == "":
			r.State = "no repo"
		case DestRefusal(paths.Real(d), protected) != "":
			r.State, r.Err = "failed", fmt.Errorf("not cloned into %s: %s", d, DestRefusal(paths.Real(d), protected))
		default:
			parent := filepath.Dir(paths.Real(d))
			if _, err := os.Stat(parent); err != nil && !paths.Within(parent, paths.Home()) {
				r.State, r.Err = "failed", fmt.Errorf("its parent folder %s is missing and outside your home folder; create it or relink the project", parent)
				break
			}
			if err := Clone(e.Get("repo"), d); err != nil {
				r.State, r.Err = "failed", err
				break
			}
			r.State = "cloned"
		}
		out = append(out, r)
	}
	return out, nil
}
