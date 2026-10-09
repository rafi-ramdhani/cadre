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
	"template/playbook.md":        {Data: []byte("# Playbook of {{name}}\n")},
	"template/projects.yaml":      {Data: []byte("# One entry per project.\n")},
	"template/cadre.conf":         {Data: []byte("PERMISSION_MODE=default\n")},
	"template/.gitignore":         {Data: []byte(".DS_Store\nnode_modules/\n")},
	"template/teams/.gitkeep":     {Data: nil},
	"template/members/dev/pm.md":  {Data: []byte("# PM of {{name}}\n")},
	"template/members/dev/eng.md": {Data: []byte("# Engineer\n")},
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
	c, note, err := Create("work", tmpl)
	if err != nil || note != "" {
		t.Fatalf("Create: %v %q", err, note)
	}
	if c.Path != home+"/.cadre/work" || !c.Present() {
		t.Errorf("cadre %+v", c)
	}
	if b, _ := os.ReadFile(c.Path + "/playbook.md"); string(b) != "# Playbook of work\n" {
		t.Errorf("name not put in: %q", b)
	}
	if b, _ := os.ReadFile(c.Path + "/.gitignore"); string(b) != ".DS_Store\nnode_modules/\n" {
		t.Errorf(".gitignore %q", b)
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
	if _, _, err := Create("work", tmpl); err == nil {
		t.Error("a second cadre with the same name was created")
	}
	if _, _, err := Create("WORK", tmpl); err == nil {
		t.Errorf("a name differing only in case: %v", err)
	}
	if _, _, err := Create("config", tmpl); err == nil {
		t.Error("a reserved name was accepted")
	}
}

func TestCreateWithoutAGitIdentity(t *testing.T) {
	fakeHome(t, false)
	c, note, err := Create("old", tmpl)
	if err != nil || !c.Present() {
		t.Fatal(err)
	}
	if !strings.Contains(note, "no user identity") {
		t.Errorf("no note without a git identity: %q", note)
	}
}

func TestList(t *testing.T) {
	home := fakeHome(t, true)
	Create("b", tmpl)
	Create("a", tmpl)
	os.MkdirAll(home+"/.cadre/notacadre", 0o755)
	os.MkdirAll(home+"/.cadre/.hidden/members", 0o755)
	os.MkdirAll(home+"/.cadre/framework/members", 0o755)
	// A config/external list from the bash 0.2.0 work is not read.
	out, _ := filepath.EvalSymlinks(t.TempDir())
	os.MkdirAll(out+"/ext/members", 0o755)
	os.WriteFile(Config("external"), []byte(out+"/ext\n"), 0o600)
	list, _ := List()
	var names []string
	for _, c := range list {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "a,b" {
		t.Errorf("list %v", names)
	}
	if Default() != "" {
		t.Error("a default appeared")
	}
	SetDefault("a")
	if Default() != "a" {
		t.Errorf("Default %q", Default())
	}
}

func TestOldCadres(t *testing.T) {
	home := fakeHome(t, true)
	Create("work", tmpl)
	old := filepath.Join(home, "Documents", "demo")
	os.MkdirAll(old+"/personas", 0o755)
	os.WriteFile(old+"/projects.yaml", nil, 0o644)
	os.MkdirAll(home+"/.config/cadre", 0o755)
	os.WriteFile(home+"/.config/cadre/home", []byte("~/Documents/demo\n"), 0o644)
	if OldHome() != old {
		t.Errorf("OldHome %q", OldHome())
	}
	if !OldCadre(old) || OldCadre(old+"/personas") || OldCadre(home+"/.cadre/work") || OldCadre(home) {
		t.Error("OldCadre")
	}
	os.WriteFile(home+"/.config/cadre/home", []byte("/nowhere/demo\n"), 0o644)
	if OldHome() != "" {
		t.Error("a missing old home was named")
	}
}

// addLink writes a project entry into a cadre's registry and records its
// place on this machine.
func addLink(t *testing.T, c Cadre, name, path string) {
	t.Helper()
	reg, _ := registry.Load(c.Registry())
	reg.Add(name, registry.Field{Key: "repo", Value: "me/" + name})
	if err := reg.Save(c.Registry()); err != nil {
		t.Fatal(err)
	}
	if err := SetPlace(c, name, expandHome(path)); err != nil {
		t.Fatal(err)
	}
}

func TestResolve(t *testing.T) {
	home := fakeHome(t, true)
	work, _, _ := Create("work", tmpl)
	life, _, _ := Create("life", tmpl)
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
	os.MkdirAll(ext+"/old/members", 0o755)

	for _, tc := range []struct{ cwd, cadre, from string }{
		{work.Path + "/teams", "work", "from this folder"},
		{ext + "/old/projects/x", "life", "default"},
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
	// Two present cadres with one name in another letter case (a file
	// system that tells them apart) are not used.
	os.MkdirAll(home+"/.cadre/WORK/members", 0o755)
	a, _ := os.Stat(home + "/.cadre/WORK")
	if b, _ := os.Stat(work.Path); !os.SameFile(a, b) {
		if _, err := Resolve(work.Path, nil); err == nil || !strings.Contains(err.Error(), "share the name") {
			t.Errorf("two cadres with one name: %v", err)
		}
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

func TestPlaces(t *testing.T) {
	home := fakeHome(t, true)
	w := Cadre{Name: "w", Path: home + "/.cadre/w"}
	e := func(text string) *registry.Entry { return registry.Parse(text).Entries()[0] }
	// A path in the registry, from another machine or an older build, is
	// not read: only this machine's places are.
	if d := ProjectDir(w, e("a:\n  repo: r\n  path: ~/Dev/a\n")); d != "" {
		t.Errorf("a registry path was read: %s", d)
	}
	SetPlace(w, "a", home+"/Dev/a")
	SetPlace(w, "b", "/elsewhere/b")
	if d := ProjectDir(w, e("a:\n  repo: r\n")); d != home+"/Dev/a" {
		t.Errorf("place: %s", d)
	}
	if raw, _ := os.ReadFile(PlacesFile(w)); !strings.Contains(string(raw), `"a": "~/Dev/a"`) || !strings.Contains(string(raw), `"b": "/elsewhere/b"`) {
		t.Errorf("places file:\n%s", raw)
	}
	if st, _ := os.Stat(PlacesFile(w)); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}
	// Another cadre has its own places.
	if Place(Cadre{Name: "v"}, "a") != "" {
		t.Error("places leaked to another cadre")
	}
	SetPlace(w, "a", "")
	if Place(w, "a") != "" || Place(w, "b") != "/elsewhere/b" {
		t.Error("forgetting a place")
	}
	// A relative entry, which cadre never writes, is ignored.
	os.WriteFile(PlacesFile(w), []byte(`{"c": "rel/c"}`), 0o600)
	if Place(w, "c") != "" {
		t.Error("a relative place was used")
	}
}

func TestWhere(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ dir, want string }{
		{"", "not here"},
		{dir, "present"},
		{dir + "/gone", "missing"},
		{"/Volumes/cadre-no-such-drive/app", "drive"},
		{"/media/someone/cadre-no-such-drive/app", "drive"},
	} {
		if got := Where(tc.dir); got != tc.want {
			t.Errorf("Where(%q) = %q, want %q", tc.dir, got, tc.want)
		}
	}
}

func TestListSkipsLinksAndBadNames(t *testing.T) {
	home := fakeHome(t, true)
	Create("work", tmpl)
	look := filepath.Join(home, "lookalike")
	os.MkdirAll(look+"/members", 0o755)
	os.Symlink(look, home+"/.cadre/linked")
	os.MkdirAll(home+"/.cadre/my.cadre/members", 0o755)
	list, _ := List()
	if len(list) != 1 || list[0].Name != "work" {
		t.Errorf("list %+v", list)
	}
}

// On a case-insensitive disk (macOS), another spelling of a cadre's folder
// or a project's folder is that folder.
func TestResolveThroughAnotherSpelling(t *testing.T) {
	home := fakeHome(t, true)
	Create("work", tmpl)
	play, _, _ := Create("play", tmpl)
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

// A places file that does not parse is refused, not replaced: replacing it
// would lose every other project's place. Entries cadre does not read are
// kept as they are.
func TestABrokenPlacesFileIsKept(t *testing.T) {
	home := fakeHome(t, true)
	w := Cadre{Name: "w", Path: home + "/.cadre/w"}
	SetPlace(w, "a", "/elsewhere/a")
	for _, bad := range []string{"not json", "null", `["a"]`} {
		os.WriteFile(PlacesFile(w), []byte(bad), 0o600)
		if err := SetPlace(w, "b", "/elsewhere/b"); err == nil || !strings.Contains(err.Error(), "fix or remove it (nothing was changed)") {
			t.Errorf("%s: %v", bad, err)
		}
		if raw, _ := os.ReadFile(PlacesFile(w)); string(raw) != bad {
			t.Errorf("%s was replaced with %s", bad, raw)
		}
	}
	os.WriteFile(PlacesFile(w), []byte(`{"x": 1, "a": "/elsewhere/a"}`), 0o600)
	if err := SetPlace(w, "b", "/elsewhere/b"); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(PlacesFile(w)); !strings.Contains(string(raw), `"x": 1`) || Place(w, "a") != "/elsewhere/a" || Place(w, "b") != "/elsewhere/b" {
		t.Errorf("places file:\n%s", raw)
	}
}
