//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
)

// bareRepo makes a local repository with one commit and returns its path,
// to clone from without a network.
func bareRepo(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), name+"-src")
	bare := filepath.Join(t.TempDir(), name+".git")
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	os.MkdirAll(src, 0o755)
	run(src, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(src, "README"), []byte(name), 0o644)
	run(src, "add", "-A")
	run(src, "commit", "-qm", "first")
	if out, err := exec.Command("git", "clone", "-q", "--bare", src, bare).CombinedOutput(); err != nil {
		t.Fatalf("bare clone: %v %s", err, out)
	}
	return bare
}

// register writes a cadre's registry from text that gives each project a
// path: the paths become this machine's places, as cadre keeps them, and
// the registry holds none.
func register(t *testing.T, home, name, text string) {
	t.Helper()
	c := cadres.Cadre{Name: name, Path: home + "/.cadre/" + name}
	var kept []string
	current := ""
	for _, l := range strings.Split(text, "\n") {
		if !strings.HasPrefix(l, " ") && strings.HasSuffix(l, ":") {
			current = strings.TrimSuffix(l, ":")
		}
		if p, ok := strings.CutPrefix(l, "  path: "); ok {
			if p == "~" || strings.HasPrefix(p, "~/") {
				p = home + p[1:]
			}
			if err := cadres.SetPlace(c, current, p); err != nil {
				t.Fatal(err)
			}
			continue
		}
		kept = append(kept, l)
	}
	if err := os.WriteFile(c.Path+"/projects.yaml", []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, _ := os.ReadFile(p)
	return string(b)
}

func TestProjectsFolderIsAskedOnce(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	repo := bareRepo(t, "app")
	// Without a terminal: refused, with the suggestion.
	refused(t, "the projects folder is not set; ask the user and run cadre project dir <folder> (suggested: ~/Developer)", "project", "add", "app", repo)
	if strings.Contains(readFile(t, home+"/.cadre/work/projects.yaml"), "app") {
		t.Error("a refused add was registered")
	}
	// On a terminal: asked, answered with the default, remembered.
	ttyForTests = true
	t.Cleanup(func() { ttyForTests = false })
	code, out, errOut := callIn("\n", "project", "add", "app", repo)
	if code != 0 || !strings.Contains(errOut, "Where do you keep your projects? [~/Developer]") {
		t.Fatalf("add on a terminal: %d %q %q", code, out, errOut)
	}
	if !strings.Contains(out, "app added, cloned to "+home+"/Developer/app") {
		t.Errorf("add: %q", out)
	}
	if got := strings.TrimSpace(readFile(t, home+"/.cadre/config/projects-dir")); got != "~/Developer" {
		t.Errorf("projects-dir %q", got)
	}
	// The registry holds no local path; this machine's places do.
	reg := readFile(t, home+"/.cadre/work/projects.yaml")
	if !strings.Contains(reg, "app:\n  repo: "+repo+"\n  team: dev\n  about:\n") || strings.Contains(reg, "path:") {
		t.Errorf("registry:\n%s", reg)
	}
	if places := readFile(t, home+"/.cadre/config/places/work.json"); !strings.Contains(places, `"app": "~/Developer/app"`) {
		t.Errorf("places:\n%s", places)
	}
	if _, err := os.Stat(home + "/.cadre/work/projects"); err == nil {
		t.Error("the add made projects/ inside the cadre")
	}
	ttyForTests = false
	if out := must(t, "project", "dir"); strings.TrimSpace(out) != "~/Developer" {
		t.Errorf("project dir: %q", out)
	}
}

func TestProjectAddTrustsAndLinks(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	must(t, "project", "dir", "~/Code")
	cfg := home + "/.claude.json"
	os.WriteFile(cfg, []byte("{\n  \"numStartups\": 1\n}\n"), 0o600)
	repo := bareRepo(t, "app")
	out := must(t, "project", "add", "app", repo, "dev", "the app")
	if !strings.Contains(out, "app added, cloned to "+home+"/Code/app and trusted in Claude Code") {
		t.Errorf("add: %q", out)
	}
	if !strings.Contains(readFile(t, cfg), `"`+home+`/Code/app": {`) || readFile(t, cfg+".bak-cadre") != "{\n  \"numStartups\": 1\n}\n" {
		t.Errorf("config %s", readFile(t, cfg))
	}
	refused(t, "already in projects.yaml", "project", "add", "app", repo)
	refused(t, "letters, digits", "project", "add", "../x", repo)
	// An existing folder that is the same repository is linked, another is refused.
	os.MkdirAll(home+"/Code/other", 0o755)
	refused(t, "is not a clone of", "project", "add", "other", repo)

	// Linking a folder the user already has.
	mine := home + "/src/mine"
	exec.Command("git", "clone", "-q", repo, mine).Run()
	out = must(t, "project", "add", "mine", "--path", mine, "research", "--no-trust")
	if !strings.Contains(out, "mine added, linked at "+mine) {
		t.Errorf("link: %q", out)
	}
	if reg := readFile(t, home+"/.cadre/work/projects.yaml"); !strings.Contains(reg, "mine:\n  repo: "+repo+"\n  team: research\n  about:\n") || strings.Contains(reg, "path:") {
		t.Errorf("registry:\n%s", reg)
	}
	if places := readFile(t, home+"/.cadre/config/places/work.json"); !strings.Contains(places, `"mine": "~/src/mine"`) {
		t.Errorf("places:\n%s", places)
	}
	refused(t, "already linked as mine", "project", "add", "again", "--path", mine)
	refused(t, "not the top folder of a git repository", "project", "add", "sub", "--path", home+"/src")
	refused(t, "inside ~/.cadre", "project", "add", "x", "--path", home+"/.cadre/work")
	refused(t, "your home folder", "project", "add", "x", "--path", home)
	t.Setenv("CADRE_PERSONA", "x")
	refused(t, "persona sessions cannot add projects", "project", "add", "y", "--path", mine)
	refused(t, "persona sessions cannot add projects", "project", "add", "z", "me/z")
	refused(t, "persona sessions cannot trust", "project", "trust", "app")
}

func TestProjectSyncAndTrust(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	must(t, "project", "dir", "~/Code")
	os.WriteFile(home+"/.claude.json", []byte("{}\n"), 0o600)
	repo := bareRepo(t, "app")
	register(t, home, "work", "app:\n  repo: "+repo+"\n  team: dev\n  about: a\nlocal:\n  team: dev\n  about: no repo\n  path: ~/nowhere/local\n")
	out := must(t, "project", "sync")
	for _, want := range []string{"app: cloned to " + home + "/Code/app", "local: not on this machine, and no repo to clone", "app: trusted in Claude Code"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync lacks %q:\n%s", want, out)
		}
	}
	if out := must(t, "project", "sync"); !strings.Contains(out, "app: present") {
		t.Errorf("second sync: %q", out)
	}
	out = must(t, "project", "trust", "--all")
	if !strings.Contains(out, "app: already trusted") || !strings.Contains(out, "local: missing locally, skipped") {
		t.Errorf("trust --all: %q", out)
	}
	refused(t, "not a registered project", "project", "trust", "nope")
}

func TestTrustRefusals(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	os.WriteFile(home+"/.claude.json", []byte("{}\n"), 0o600)
	register(t, home, "work", "me:\n  path: ~\nteam:\n  path: ~/.cadre/work/teams/dev\nnotgit:\n  path: ~/plain\n")
	os.MkdirAll(home+"/.cadre/work/teams/dev", 0o755)
	os.MkdirAll(home+"/plain", 0o755)
	out := must(t, "project", "trust", "--all")
	for _, want := range []string{"me: not trusted, it is your home folder", "team: not trusted, it is inside ~/.cadre", "notgit: not trusted, it is not the top folder of a git repo"} {
		if !strings.Contains(out, want) {
			t.Errorf("trust --all lacks %q:\n%s", want, out)
		}
	}
	if readFile(t, home+"/.claude.json") != "{}\n" {
		t.Error("a refused trust changed the config")
	}
}

// A shared registry's paths are checked like links: a project cannot be
// cloned where Claude Code or cadre load files.
func TestSyncAndAddRefuseCadresAndClaudesFolders(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	repo := bareRepo(t, "demo")
	register(t, home, "work", "demo:\n  repo: "+repo+"\n  path: ~/.claude/skills/demo\nmine:\n  repo: "+repo+"\n  path: ~/.cadre/config/x\n")
	code, out, _ := call("project", "sync")
	if code == 0 || !strings.Contains(out, "demo: not cloned: not cloned into "+home+"/.claude/skills/demo: it is inside ~/.claude") || !strings.Contains(out, "mine: not cloned") {
		t.Errorf("sync: %d %q", code, out)
	}
	for _, p := range []string{home + "/.claude/skills/demo", home + "/.cadre/config/x"} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s was made", p)
		}
	}
	refused(t, "cannot hold projects: it is inside ~/.claude", "project", "dir", "~/.claude/skills")
	refused(t, "cannot hold projects: it is inside ~/.cadre", "project", "dir", "~/.cadre/x")
	// A repo that looks like an option never reaches git: the command line
	// refuses it, and Clone refuses it too (its own test).
	refused(t, "unknown option --upload-pack", "project", "add", "evil", "--upload-pack=touch /tmp/x")
}

// Projects across machines: the registry is portable, the places are this
// machine's, and a missing project is never unlinked by cadre.
func TestProjectLinkUnlinkAndMissing(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	must(t, "init", "work")
	must(t, "project", "dir", "~/Code")
	repo := bareRepo(t, "app")
	other := bareRepo(t, "other")
	must(t, "project", "add", "app", repo, "--no-trust")
	// The folder moves away: missing, and nothing unlinks it.
	os.MkdirAll(home+"/Moved", 0o755)
	os.Rename(home+"/Code/app", home+"/Moved/app")
	if out := must(t, "ls"); !strings.Contains(out, "projects: app (missing: app)") {
		t.Errorf("ls:\n%s", out)
	}
	refused(t, "project 'app' is missing", "up", "dev/engineer", "app")
	refused(t, "project 'app' is missing", "attach", "dev", "app")
	refused(t, "project 'app' is missing", "project", "path", "app")
	// A clone of the same repo in the projects folder is suggested.
	exec.Command("git", "clone", "-q", repo, home+"/Code/app-copy").Run()
	var st cadreStatus
	json.Unmarshal([]byte(must(t, "ls", "--json")), &st)
	if len(st.Projects) != 1 || st.Projects[0].State != "missing" || st.Projects[0].Found != home+"/Code/app-copy" {
		t.Errorf("ls --json: %+v", st.Projects)
	}
	// Link it where it is now.
	exec.Command("git", "clone", "-q", other, home+"/Moved/other").Run()
	refused(t, "is a clone of", "project", "link", "app", home+"/Moved/other")
	refused(t, "not a registered project", "project", "link", "nope", home+"/Moved/app")
	refused(t, "your home folder", "project", "link", "app", home)
	out := must(t, "project", "link", "app", home+"/Moved/app", "--no-trust")
	if !strings.Contains(out, "app is at "+home+"/Moved/app on this machine") || strings.TrimSpace(must(t, "project", "path", "app")) != home+"/Moved/app" {
		t.Errorf("link: %q", out)
	}
	if strings.Contains(readFile(t, home+"/.cadre/work/projects.yaml"), "Moved") {
		t.Error("a local path went into the registry")
	}
	// Personas cannot change links.
	t.Setenv("CADRE_PERSONA", "x")
	refused(t, "persona sessions cannot link folders", "project", "link", "app", home+"/Moved/app")
	refused(t, "persona sessions cannot unlink projects", "project", "unlink", "app")
	t.Setenv("CADRE_PERSONA", "")
	// Unlink: out of the registry and the places, the folder kept, and
	// with --untrust out of the runtime's trust too.
	os.WriteFile(home+"/.claude.json", []byte("{}\n"), 0o600)
	must(t, "project", "trust", "app")
	out = must(t, "project", "unlink", "app", "--untrust")
	if !strings.Contains(out, "app unlinked; its folder ~/Moved/app is kept") || !strings.Contains(out, "app: untrusted") {
		t.Errorf("unlink: %q", out)
	}
	if _, err := os.Stat(home + "/Moved/app/.git"); err != nil {
		t.Error("unlink touched the folder")
	}
	if strings.Contains(readFile(t, home+"/.cadre/work/projects.yaml"), "app:") || strings.Contains(readFile(t, home+"/.cadre/config/places/work.json"), "app") {
		t.Error("app is still registered or placed")
	}
	if strings.Contains(readFile(t, home+"/.claude.json"), "hasTrustDialogAccepted\": true") {
		t.Error("--untrust left the trust")
	}
	if log, _ := exec.Command("git", "-C", home+"/.cadre/work", "log", "-1", "--format=%s").Output(); strings.TrimSpace(string(log)) != "Remove project app" {
		t.Errorf("commit %q", log)
	}
	refused(t, "not a registered project", "project", "unlink", "app")
}

// A registry from another machine: sync clones what is not here and uses a
// clone of the same repository already in the projects folder.
func TestSyncPlacesProjects(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	must(t, "project", "dir", "~/Code")
	app, blog := bareRepo(t, "app"), bareRepo(t, "blog")
	os.WriteFile(home+"/.cadre/work/projects.yaml", []byte("app:\n  repo: "+app+"\nblog:\n  repo: "+blog+"\n  path: /other/machine/blog\n"), 0o644)
	exec.Command("git", "clone", "-q", app, home+"/Code/app").Run()
	if out := must(t, "ls"); !strings.Contains(out, "(not on this machine: app, blog)") {
		t.Errorf("ls before sync:\n%s", out)
	}
	out := must(t, "project", "sync", "--no-trust")
	if !strings.Contains(out, "app: already at "+home+"/Code/app") || !strings.Contains(out, "blog: cloned to "+home+"/Code/blog") {
		t.Errorf("sync: %q", out)
	}
	places := readFile(t, home+"/.cadre/config/places/work.json")
	if !strings.Contains(places, `"app": "~/Code/app"`) || !strings.Contains(places, `"blog": "~/Code/blog"`) {
		t.Errorf("places:\n%s", places)
	}
	if out := must(t, "ls"); !strings.Contains(out, "projects: app, blog\n") {
		t.Errorf("ls after sync:\n%s", out)
	}
}

// A registry can come from another machine or a shared backup, and a
// member with a shell can write it: a name that climbs out of the projects
// folder or nests is left out everywhere, with a warning, and nothing is
// cloned for it, whoever runs sync.
func TestRegistryNamesNeverLeaveTheProjectsFolder(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	must(t, "project", "dir", "~/Code")
	app := bareRepo(t, "app")
	bad := []string{"../.vim/pack/x/start/evil", "a/b", "/abs", "~/evil", ".hidden", "x..y"}
	var text string
	for _, n := range bad {
		text += n + ":\n  repo: " + app + "\n"
	}
	os.WriteFile(home+"/.cadre/work/projects.yaml", []byte(text+"good:\n  repo: "+app+"\n"), 0o644)
	for _, member := range []string{"work-dev-engineer", ""} {
		t.Setenv("CADRE_PERSONA", member)
		code, out, errOut := call("project", "sync", "--no-trust")
		if code != 0 || !strings.Contains(out, "good: ") || strings.Contains(out, "evil") || strings.Contains(out, "a/b") {
			t.Errorf("sync (member %q): %d\n%s%s", member, code, out, errOut)
		}
		if !strings.Contains(errOut, `projects.yaml has an entry named "../.vim/pack/x/start/evil", which is not a project name`) {
			t.Errorf("no warning: %q", errOut)
		}
	}
	for _, p := range []string{home + "/.vim", home + "/Code/a", home + "/Code/abs", home + "/evil", home + "/Code/.hidden", home + "/Code/x..y"} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s was made", p)
		}
	}
	if places := readFile(t, home+"/.cadre/config/places/work.json"); strings.Contains(places, "evil") || !strings.Contains(places, `"good"`) {
		t.Errorf("places:\n%s", places)
	}
	if out := must(t, "ls"); !strings.Contains(out, `entry named "a/b", which is not a project name`) {
		t.Errorf("ls:\n%s", out)
	}
	refused(t, "", "project", "path", "../.vim/pack/x/start/evil")
	refused(t, "", "project", "link", "a/b", home+"/Code/good")
}

// A folder whose origin only seems to name the project's host is not a
// clone of it: git reads the host as evil.example in each of these.
func TestLinkRefusesAnOriginThatHidesItsHost(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	os.WriteFile(home+"/.cadre/work/projects.yaml", []byte("tool:\n  repo: acme/tool\n"), 0o644)
	for i, origin := range []string{"https://evil.example#@github.com/acme/tool", "https://evil.example?@github.com/acme/tool", `https://evil.example\@github.com/acme/tool`} {
		dir := filepath.Join(home, "src", "tool"+string(rune('a'+i)))
		os.MkdirAll(dir, 0o755)
		exec.Command("git", "-C", dir, "init", "-q").Run()
		exec.Command("git", "-C", dir, "remote", "add", "origin", origin).Run()
		refused(t, "is a clone of", "project", "link", "tool", dir, "--no-trust")
	}
	if places := readFile(t, home+"/.cadre/config/places/work.json"); strings.Contains(places, "tool") {
		t.Errorf("a place was recorded:\n%s", places)
	}
}
