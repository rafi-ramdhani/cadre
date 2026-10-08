package cadres

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rafi-ramdhani/cadre/internal/registry"
)

var tmpl = fstest.MapFS{
	"template/playbook.md":         {Data: []byte("# Playbook of {{name}}\n")},
	"template/projects.yaml":       {Data: []byte("# One entry per project.\n")},
	"template/cadre.conf":          {Data: []byte("PERMISSION_MODE=default\n")},
	"template/.gitignore":          {Data: []byte(".DS_Store\n# Project repos live here but are their own git repos; projects.yaml links them.\n/projects/\n")},
	"template/teams/.gitkeep":      {Data: nil},
	"template/personas/dev/pm.md":  {Data: []byte("# PM of {{name}}\n")},
	"template/personas/dev/eng.md": {Data: []byte("# Engineer\n")},
}

// fakeHome gives the test a HOME of its own, with git set up there.
func fakeHome(t *testing.T, identity bool) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("CADRE_HOME", "")
	gitconfig := filepath.Join(home, ".gitconfig-test")
	body := "[init]\n\tdefaultBranch = main\n"
	if identity {
		body += "[user]\n\tname = T\n\temail = t@example.com\n"
	}
	os.WriteFile(gitconfig, []byte(body), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return home
}

func TestCheckName(t *testing.T) {
	for _, ok := range []string{"work", "Work-2", "a_b"} {
		if err := CheckName(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", ".hidden", "my cadre", "a.b", "x/y", "config", "Framework", "-x"} {
		if err := CheckName(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCreateInCadreFolder(t *testing.T) {
	home := fakeHome(t, true)
	c, note, err := Create("work", "", tmpl)
	if err != nil || note != "" {
		t.Fatalf("Create: %v %q", err, note)
	}
	if c.Path != home+"/.cadre/work" || c.External || !c.Present() {
		t.Errorf("cadre %+v", c)
	}
	if b, _ := os.ReadFile(c.Path + "/playbook.md"); string(b) != "# Playbook of work\n" {
		t.Errorf("name not put in: %q", b)
	}
	if b, _ := os.ReadFile(c.Path + "/.gitignore"); strings.Contains(string(b), "projects") {
		t.Errorf(".gitignore keeps projects/: %q", b)
	}
	if _, err := os.Stat(c.Path + "/projects"); err == nil {
		t.Error("a cadre in ~/.cadre has projects/")
	}
	if _, err := os.Stat(c.Path + "/teams/.gitkeep"); err != nil {
		t.Error("dot files of the template were not copied")
	}
	if out, _ := exec.Command("git", "-C", c.Path, "log", "--format=%s").Output(); strings.TrimSpace(string(out)) != "Start work from the cadre template" {
		t.Errorf("commit %q", out)
	}
	if _, _, err := Create("work", "", tmpl); err == nil {
		t.Error("a second cadre with the same name was created")
	}
	if _, _, err := Create("WORK", t.TempDir(), tmpl); err == nil || !strings.Contains(err.Error(), "already at") {
		t.Errorf("a name differing only in case: %v", err)
	}
	if _, _, err := Create("config", "", tmpl); err == nil {
		t.Error("a reserved name was accepted")
	}
}

func TestCreateOutside(t *testing.T) {
	fakeHome(t, false)
	parent, _ := filepath.EvalSymlinks(t.TempDir())
	c, note, err := Create("old", parent, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if !c.External || c.Path != parent+"/old" {
		t.Errorf("cadre %+v", c)
	}
	if !strings.Contains(note, "no user identity") {
		t.Errorf("no note without a git identity: %q", note)
	}
	if _, err := os.Stat(c.Path + "/projects"); err != nil {
		t.Error("an outside cadre has no projects/")
	}
	if b, _ := os.ReadFile(c.Path + "/.gitignore"); !strings.Contains(string(b), "/projects/") {
		t.Error("an outside cadre's .gitignore lost projects/")
	}
	list, _ := List()
	if len(list) != 1 || list[0].Path != c.Path || !list[0].External {
		t.Errorf("list %+v", list)
	}
}

func TestListAndExternal(t *testing.T) {
	home := fakeHome(t, true)
	Create("b", "", tmpl)
	Create("a", "", tmpl)
	os.MkdirAll(home+"/.cadre/notacadre", 0o755)
	os.MkdirAll(home+"/.cadre/.hidden/personas", 0o755)
	os.MkdirAll(home+"/.cadre/framework/personas", 0o755)
	out, _ := filepath.EvalSymlinks(t.TempDir())
	os.MkdirAll(out+"/ext/personas", 0o755)
	AddExternal(out + "/ext")
	AddExternal(out + "/ext") // once only
	os.WriteFile(Config("external"), []byte(out+"/ext\n  relative/x\n~/y\n"+out+"/gone\n"), 0o600)
	list, _ := List()
	var names []string
	for _, c := range list {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "a,b,ext,gone" {
		t.Errorf("list %v", names)
	}
	if c, ok := Find("gone"); !ok || c.Present() {
		t.Errorf("a missing outside cadre: %+v %v", c, ok)
	}
	RemoveExternal(out + "/gone")
	if _, ok := Find("gone"); ok {
		t.Error("RemoveExternal kept it")
	}
	if Default() != "" {
		t.Error("a default appeared")
	}
	SetDefault("a")
	if Default() != "a" {
		t.Errorf("Default %q", Default())
	}
}

func TestCopyOldConfig(t *testing.T) {
	home := fakeHome(t, true)
	visible := filepath.Join(home, "Documents", "demo")
	other := filepath.Join(home, "Documents", "two")
	os.MkdirAll(visible+"/personas", 0o755)
	os.MkdirAll(other+"/personas", 0o755)
	old := OldConfig()
	os.MkdirAll(old, 0o755)
	os.WriteFile(old+"/home", []byte("~/Documents/demo\n"), 0o644)
	os.WriteFile(old+"/cadres", []byte(visible+"\n"+other+"\n"), 0o644)
	os.WriteFile(old+"/persona-settings.sha256", []byte("abc "+visible+"/.claude/persona-settings.json\n"), 0o600)
	os.MkdirAll(old+"/cadres.lock", 0o755)
	msg, err := CopyOldConfig()
	if err != nil || msg == "" {
		t.Fatalf("CopyOldConfig: %q %v", msg, err)
	}
	if Default() != "demo" {
		t.Errorf("default %q", Default())
	}
	if b, _ := os.ReadFile(Config("external")); string(b) != visible+"\n"+other+"\n" {
		t.Errorf("external %q", b)
	}
	if b, _ := os.ReadFile(Config("persona-settings.sha256")); !strings.Contains(string(b), "abc ") {
		t.Errorf("fingerprints %q", b)
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Error("~/.config/cadre is still there")
	}
	if _, err := os.Stat(old + ".moved-to-0.2.0/home"); err != nil || !strings.Contains(msg, ".moved-to-0.2.0") {
		t.Errorf("the old folder was not kept aside: %v %q", err, msg)
	}
	if msg, _ := CopyOldConfig(); msg != "" {
		t.Error("a second copy did something")
	}
	if r, err := Resolve(home, nil); err != nil || r.Path != visible || r.From != "default" {
		t.Errorf("resolve after the copy: %+v %v", r, err)
	}
}

// addLink writes a project entry into a cadre's registry.
func addLink(t *testing.T, c Cadre, name, path string) {
	t.Helper()
	reg, _ := registry.Load(c.Registry())
	reg.Add(name, registry.Field{Key: "repo", Value: "me/" + name}, registry.Field{Key: "path", Value: path})
	if err := reg.Save(c.Registry()); err != nil {
		t.Fatal(err)
	}
}

func TestResolve(t *testing.T) {
	home := fakeHome(t, true)
	work, _, _ := Create("work", "", tmpl)
	life, _, _ := Create("life", "", tmpl)
	SetDefault("life")
	dev := filepath.Join(home, "Developer")
	for _, d := range []string{"app/src", "app/sub/inner", "shared", "unlinked"} {
		os.MkdirAll(filepath.Join(dev, d), 0o755)
	}
	addLink(t, work, "app", "~/Developer/app")
	addLink(t, work, "inner", dev+"/app/sub/inner")
	addLink(t, work, "shared", "~/Developer/shared")
	addLink(t, life, "shared", "~/Developer/shared")
	ext, _ := filepath.EvalSymlinks(t.TempDir())
	os.MkdirAll(ext+"/old/personas", 0o755)
	AddExternal(ext + "/old")

	for _, tc := range []struct{ cwd, cadre, from string }{
		{work.Path + "/teams", "work", "from this folder"},
		{ext + "/old/projects/x", "old", "from this folder"},
		{dev + "/app/src", "work", "from the project app"},
		{dev + "/app/sub/inner", "work", "from the project inner"},
		{dev + "/unlinked", "life", "default"},
		{home, "life", "default"},
	} {
		os.MkdirAll(tc.cwd, 0o755)
		r, err := Resolve(tc.cwd, nil)
		if err != nil || r.Name != tc.cadre || r.From != tc.from || r.Default != "life" {
			t.Errorf("Resolve(%s) = %+v, %v; want %s %s", tc.cwd, r, err, tc.cadre, tc.from)
		}
	}
	// One project, two cadres: refused without a terminal, asked with one.
	_, err := Resolve(dev+"/shared", nil)
	if err == nil || err.Error() != "shared is linked by life and work; run from ~/.cadre/<name>, or set CADRE_HOME" {
		t.Errorf("two cadres without a terminal: %v", err)
	}
	r, err := Resolve(dev+"/shared", func(project string, names []string) (string, error) { return "work", nil })
	if err != nil || r.Name != "work" || r.Project != "shared" {
		t.Errorf("asked: %+v %v", r, err)
	}
	if got := Linking(dev + "/shared"); len(got) != 2 {
		t.Errorf("Linking: %+v", got)
	}
	// CADRE_HOME wins wherever the command runs, registered or not.
	t.Setenv("CADRE_HOME", ext+"/old")
	if r, _ := Resolve(work.Path, nil); r.Name != "old" || r.From != "from CADRE_HOME" {
		t.Errorf("CADRE_HOME: %+v", r)
	}
	t.Setenv("CADRE_HOME", "")
	// Two present cadres with one name are not used.
	os.MkdirAll(ext+"/WORK/personas", 0o755)
	AddExternal(ext + "/WORK")
	if _, err := Resolve(work.Path, nil); err == nil || !strings.Contains(err.Error(), "share the name") {
		t.Errorf("two cadres with one name: %v", err)
	}
}

func TestResolveWithNothing(t *testing.T) {
	home := fakeHome(t, true)
	if _, err := Resolve(home, nil); !errors.Is(err, ErrNoCadre) {
		t.Errorf("no cadre: %v", err)
	}
	SetDefault("gone")
	if _, err := Resolve(home, nil); !errors.Is(err, ErrNoCadre) {
		t.Errorf("a missing default: %v", err)
	}
}

func TestProjectDir(t *testing.T) {
	home := fakeHome(t, true)
	in := Cadre{Name: "w", Path: home + "/.cadre/w"}
	out := Cadre{Name: "o", Path: "/x/o", External: true}
	e := func(text string) *registry.Entry { return registry.Parse(text).Entries()[0] }
	if d := ProjectDir(in, e("a:\n  path: ~/Dev/a\n")); d != home+"/Dev/a" {
		t.Errorf("~ path: %s", d)
	}
	if d := ProjectDir(out, e("a:\n  path: elsewhere/a\n")); d != "/x/o/elsewhere/a" {
		t.Errorf("relative path: %s", d)
	}
	if d := ProjectDir(out, e("a:\n  repo: r\n")); d != "/x/o/projects/a" {
		t.Errorf("outside cadre, no path: %s", d)
	}
	if d := ProjectDir(in, e("a:\n  repo: r\n")); d != "" {
		t.Errorf("no projects folder set: %s", d)
	}
	os.MkdirAll(ConfigDir(), 0o700)
	os.WriteFile(Config("projects-dir"), []byte("~/Developer\n"), 0o600)
	if d := ProjectDir(in, e("a:\n  repo: r\n")); d != home+"/Developer/a" {
		t.Errorf("projects folder: %s", d)
	}
}

func TestListSkipsLinksAndBadNames(t *testing.T) {
	home := fakeHome(t, true)
	Create("work", "", tmpl)
	look := filepath.Join(home, "lookalike")
	os.MkdirAll(look+"/personas", 0o755)
	os.Symlink(look, home+"/.cadre/linked")
	os.MkdirAll(home+"/.cadre/my.cadre/personas", 0o755)
	list, _ := List()
	if len(list) != 1 || list[0].Name != "work" {
		t.Errorf("list %+v", list)
	}
}

// On a case-insensitive disk (macOS), another spelling of a cadre's folder
// or a project's folder is that folder.
func TestResolveThroughAnotherSpelling(t *testing.T) {
	home := fakeHome(t, true)
	Create("work", "", tmpl)
	play, _, _ := Create("play", "", tmpl)
	SetDefault("work")
	upper := home + "/.CADRE/PLAY/teams"
	if _, err := os.Stat(upper); err != nil {
		t.Skip("the disk is case-sensitive")
	}
	if r, err := Resolve(upper, nil); err != nil || r.Name != "play" || r.From != "from this folder" {
		t.Errorf("a case variant of a cadre folder: %+v %v", r, err)
	}
	os.MkdirAll(home+"/Dev/app", 0o755)
	addLink(t, play, "app", "~/Dev/app")
	if r, err := Resolve(home+"/DEV/APP", nil); err != nil || r.Name != "play" || r.Project != "app" {
		t.Errorf("a case variant of a project folder: %+v %v", r, err)
	}
	t.Setenv("CADRE_HOME", home+"/.cadre/PLAY")
	if r, _ := Resolve(home, nil); r.Name != "play" {
		t.Errorf("CADRE_HOME in another case: %+v", r)
	}
}
