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

// Add registers a project and clones it into the projects folder, and
// records that folder as the project's place on this machine. A folder
// already there is used when it is the same repository and refused
// otherwise. The clone comes first, so a failed clone registers nothing.
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
	dir := filepath.Join(projectsDir, s.Name)
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
	reg.Add(s.Name, registry.Field{Key: "repo", Value: s.Repo}, registry.Field{Key: "team", Value: s.Team},
		registry.Field{Key: "about", Value: s.About})
	if err := reg.Save(c.Registry()); err != nil {
		return a, err
	}
	if err := cadres.SetPlace(c, s.Name, dir); err != nil {
		return a, err
	}
	note, err := cadres.Commit(c.Path, "Add project "+s.Name, "projects.yaml")
	if note != "" {
		a.Notes = append(a.Notes, note)
	}
	return a, err
}

// LinkRefusal says why a folder (physical) cannot be linked as a project,
// or "".
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
		if paths.Within(dir, c.Path) {
			return "it is inside the cadre " + c.Name
		}
	}
	return ""
}

// Link registers a folder the user already has: it must be the top folder
// of a git repository and not cadre's own. The repo is the one given, else
// the folder's origin. The folder is recorded as the project's place on
// this machine, never in the registry.
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
		registry.Field{Key: "about", Value: s.About})
	if err := reg.Save(c.Registry()); err != nil {
		return a, err
	}
	if err := cadres.SetPlace(c, s.Name, phys); err != nil {
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
	Name, State, Dir string // State: "present", "cloned", "found", "no repo", "no folder", "drive", "failed"
	Err              error
}

// Sync gives every registry project with a repo a folder on this machine:
// it clones each one that has none, to its recorded place, else into the
// projects folder (projectsDir, "" when not set), and records the place.
// A clone of the same repository already there is used as it is. A place
// on a drive that is not connected is left alone. Parent folders are made
// under the home folder only.
func Sync(c cadres.Cadre, projectsDir string, protected []string) ([]SyncResult, error) {
	reg, err := registry.Load(c.Registry())
	if err != nil {
		return nil, err
	}
	var out []SyncResult
	for _, e := range reg.Entries() {
		d := cadres.ProjectDir(c, e)
		where := cadres.Where(d)
		if d == "" && projectsDir != "" {
			d = filepath.Join(projectsDir, e.Name)
		}
		r := SyncResult{Name: e.Name, Dir: d}
		st, statErr := os.Stat(d)
		switch {
		case where == "present":
			r.State = "present"
		case where == "drive":
			r.State = "drive"
		case e.Get("repo") == "":
			r.State = "no repo"
		case d == "":
			r.State = "no folder"
		case DestRefusal(paths.Real(d), protected) != "":
			r.State, r.Err = "failed", fmt.Errorf("not cloned into %s: %s", d, DestRefusal(paths.Real(d), protected))
		case statErr == nil && st.IsDir():
			if !SameRepo(Origin(d), e.Get("repo")) {
				r.State, r.Err = "failed", fmt.Errorf("%s exists and is not a clone of %s; link the project's folder with cadre project link %s <dir>", d, e.Get("repo"), e.Name)
				break
			}
			r.State = "found"
		default:
			parent := filepath.Dir(paths.Real(d))
			if _, err := os.Stat(parent); err != nil && !paths.Within(parent, paths.Home()) {
				r.State, r.Err = "failed", fmt.Errorf("its parent folder %s is missing and outside your home folder; create it or link the project elsewhere", parent)
				break
			}
			if err := Clone(e.Get("repo"), d); err != nil {
				r.State, r.Err = "failed", err
				break
			}
			r.State = "cloned"
		}
		if r.State == "cloned" || r.State == "found" {
			if err := cadres.SetPlace(c, e.Name, d); err != nil {
				r.State, r.Err = "failed", err
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// Relink sets where a registered project is on this machine: a folder
// that moved, or a clone made somewhere else. The folder must be the top
// of a git repository, not cadre's own, and, when both are known, a clone
// of the project's repo.
func Relink(c cadres.Cadre, name, dir string, protected []string) (string, error) {
	reg, err := registry.Load(c.Registry())
	if err != nil {
		return "", err
	}
	e := reg.Get(name)
	if e == nil {
		return "", fmt.Errorf("'%s' is not a registered project (see cadre ls)", name)
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("%s is not a folder", dir)
	}
	phys := paths.Real(dir)
	if why := LinkRefusal(phys, protected); why != "" {
		return "", fmt.Errorf("%s cannot be linked: %s", dir, why)
	}
	if repo, origin := e.Get("repo"), Origin(phys); repo != "" && origin != "" && !SameRepo(origin, repo) {
		return "", fmt.Errorf("%s is a clone of %s, not of %s", dir, origin, repo)
	}
	for _, other := range reg.Entries() {
		if other.Name != name && cadres.ProjectDir(c, other) != "" && paths.Real(cadres.ProjectDir(c, other)) == phys {
			return "", fmt.Errorf("%s is already linked as %s", dir, other.Name)
		}
	}
	return phys, cadres.SetPlace(c, name, phys)
}

// Unlink removes a project from the registry and forgets its place on
// this machine. Its folder is never touched. It returns the folder it
// had, for untrusting.
func Unlink(c cadres.Cadre, name string) (dir string, notes []string, err error) {
	reg, err := registry.Load(c.Registry())
	if err != nil {
		return "", nil, err
	}
	e := reg.Get(name)
	if e == nil {
		return "", nil, fmt.Errorf("'%s' is not a registered project (see cadre ls)", name)
	}
	dir = cadres.ProjectDir(c, e)
	reg.Remove(name)
	if err := reg.Save(c.Registry()); err != nil {
		return "", nil, err
	}
	if err := cadres.SetPlace(c, name, ""); err != nil {
		return "", nil, err
	}
	note, err := cadres.Commit(c.Path, "Remove project "+name, "projects.yaml")
	if note != "" {
		notes = append(notes, note)
	}
	return dir, notes, err
}

// Found returns a folder in the projects folder whose origin is repo, for
// a project missing on this machine, or "".
func Found(projectsDir, repo string) string {
	if projectsDir == "" || repo == "" {
		return ""
	}
	entries, _ := os.ReadDir(projectsDir)
	for _, d := range entries {
		p := filepath.Join(projectsDir, d.Name())
		if d.IsDir() && SameRepo(Origin(p), repo) {
			return p
		}
	}
	return ""
}
