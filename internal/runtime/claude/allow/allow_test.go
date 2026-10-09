package allow

import (
	"bufio"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// layout builds the fake HOME the golden outcomes were recorded in (see
// testdata/golden.tsv): a cadrei named demo, a second known cadrei, a symlink
// into the cadrei, and for "stow", ~/.config and ~/.ssh as symlinks into
// ~/dotfiles.
func layout(t *testing.T, kind string) (*Checker, map[string]string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := root + "/home"
	cadrei := home + "/work/demo"
	other := root + "/multi/b"
	for _, d := range []string{cadrei + "/.claude", cadrei + "/members", other + "/members", home + "/Documents"} {
		os.MkdirAll(d, 0o755)
	}
	if kind == "stow" {
		for _, d := range []string{"config/cadre", "config/git", "config/fish", "ssh"} {
			os.MkdirAll(home+"/dotfiles/"+d, 0o755)
		}
		os.Symlink(home+"/dotfiles/config", home+"/.config")
		os.Symlink(home+"/dotfiles/ssh", home+"/.ssh")
	} else {
		os.MkdirAll(home+"/.config/cadre", 0o755)
		os.MkdirAll(home+"/.ssh", 0o755)
	}
	os.Symlink(cadrei, home+"/Documents/link")
	c := &Checker{Root: home + "/.cadrei", Home: home, Cache: home + "/.cache", Cadrei: cadrei, Cadreis: []string{cadrei, cadrei, other}}
	return c, map[string]string{"{CADREI}": cadrei, "{OTHER}": other, "{PARENT}": home + "/work", "{HOME}": home, "{ROOT}": root}
}

// fill puts the layout's paths into a placeholder text, and mask takes
// them out again, longest first.
func fill(s string, ph map[string]string) string {
	for _, k := range []string{"{CADREI}", "{OTHER}", "{PARENT}", "{HOME}", "{ROOT}"} {
		s = strings.ReplaceAll(s, k, ph[k])
	}
	return s
}

func mask(s string, ph map[string]string) string {
	keys := make([]string, 0, len(ph))
	for k := range ph {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(ph[keys[i]]) > len(ph[keys[j]]) })
	for _, k := range keys {
		s = strings.ReplaceAll(s, ph[k], k)
	}
	return s
}

func check(c *Checker, kind, input string) (outcome, message string) {
	var warn string
	var err error
	if kind == "A" {
		warn, err = c.Auto(input)
	} else {
		warn, err = c.Rule(input)
	}
	switch {
	case err != nil:
		return "refuse", err.Error()
	case warn != "":
		return "warn", warn
	}
	return "accept", ""
}

func readTSV(t *testing.T, name string) [][]string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows [][]string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if line := sc.Text(); line != "" && !strings.HasPrefix(line, "#") {
			rows = append(rows, strings.Split(line, "\t"))
		}
	}
	return rows
}

// differences maps an input to the outcome the Go checker deliberately has.
func differences(t *testing.T) map[string][]string {
	out := map[string][]string{}
	for _, r := range readTSV(t, "differences.tsv") {
		out[r[0]+"\t"+r[1]] = r
	}
	return out
}

// TestGolden runs every input from the bash reviews and smoke.sh and
// expects the bash checker's outcome and message, in both layouts, apart
// from the listed differences.
func TestGolden(t *testing.T) {
	rows := readTSV(t, "golden.tsv")
	if len(rows) < 400 {
		t.Fatalf("only %d golden rows", len(rows))
	}
	diff := differences(t)
	for _, kind := range []string{"plain", "stow"} {
		c, ph := layout(t, kind)
		for _, r := range rows {
			source, k, input := r[0], r[1], r[2]
			want, wantMsg := r[3], r[4]
			if kind == "stow" {
				want, wantMsg = r[5], r[6]
			}
			got, msg := check(c, k, fill(input, ph))
			msg = mask(msg, ph)
			if d, ok := diff[k+"\t"+input]; ok {
				if got != d[2] || !strings.Contains(msg, d[3]) {
					t.Errorf("[%s] %s (difference: %s): got %s %q, want %s with %q", kind, input, d[4], got, msg, d[2], d[3])
				}
				continue
			}
			if got != want || msg != wantMsg {
				t.Errorf("[%s] %s (from %s):\n  got  %s %q\n  want %s %q", kind, input, source, got, msg, want, wantMsg)
			}
		}
	}
}

// TestDifferences checks the inputs that only the differences list has.
func TestDifferences(t *testing.T) {
	c, ph := layout(t, "plain")
	for _, d := range readTSV(t, "differences.tsv") {
		got, msg := check(c, d[0], fill(d[1], ph))
		msg = mask(msg, ph)
		if got != d[2] || !strings.Contains(msg, d[3]) {
			t.Errorf("%s (%s): got %s %q, want %s with %q", d[1], d[4], got, msg, d[2], d[3])
		}
	}
}

func TestNorm(t *testing.T) {
	for in, want := range map[string]string{
		"CADREI":        "cadrei",
		"c\u200badrei":  "cadrei", // a zero-width space (Cf) is dropped
		"member\u2011x": "member-x",
		"a\u00a0b":      "a b",
		"ｃａｄｒｅｉ":        "cadrei", // fullwidth letters fold under NFKC
		"Straße":        "strasse",
		"ﬁle":           "file",
		"\u2014":        "-",
	} {
		if got := normText(in); got != want {
			t.Errorf("normText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewReadsTheEnvironment(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")
	c := New(home+"/work/demo", []string{"/x/other"})
	if c.Home != home || c.Cache != home+"/.cache" || c.Cadrei != home+"/work/demo" || len(c.Cadreis) != 2 {
		t.Errorf("New: %+v", c)
	}
	t.Setenv("XDG_CACHE_HOME", home+"/cache")
	if c := New(home, nil); c.Cache != home+"/cache" {
		t.Errorf("XDG_CACHE_HOME not used: %q", c.Cache)
	}
}

func FuzzRule(f *testing.F) {
	for _, s := range []string{"Bash(npm test)", "Edit(~/.ss[h]/config)", "Edit(//x/{a,b}/c)", "Read(~/.ssh\\/x)", "Bash(a \\\\; b *)"} {
		f.Add(s)
	}
	// A real folder: on macOS /home is an automount point, where resolving
	// a path can block.
	home, _ := filepath.EvalSymlinks(f.TempDir())
	c := &Checker{Home: home, Cache: home + "/.cache", Cadrei: home + "/work/demo", Cadreis: []string{home + "/work/demo"}}
	f.Fuzz(func(t *testing.T, rule string) {
		c.Rule(rule)
		c.Auto(rule)
	})
}

// TestNeverWeakerThanBash runs the review kit's 12,000 generated rules
// (dfz-rules.txt), recorded with the bash 0.2.0 checker's outcome in
// testdata/dfz.tsv. The Go checker may be stricter, never weaker: what bash
// refused, Go refuses; what bash warned about, Go warns about or refuses.
func TestNeverWeakerThanBash(t *testing.T) {
	rows := readTSV(t, "dfz.tsv")
	if len(rows) < 12000 {
		t.Fatalf("only %d rows", len(rows))
	}
	c, ph := layout(t, "plain")
	rank := map[string]int{"accept": 0, "warn": 1, "refuse": 2}
	weaker, stricter := 0, 0
	for _, r := range rows {
		want, rule := r[0], r[1]
		got, msg := check(c, "R", fill(rule, ph))
		switch {
		case rank[got] < rank[want]:
			weaker++
			if weaker <= 20 {
				t.Errorf("%s: bash %s, Go %s %q", rule, want, got, mask(msg, ph))
			}
		case rank[got] > rank[want]:
			stricter++
		}
	}
	t.Logf("%d rules: %d weaker, %d stricter than bash", len(rows), weaker, stricter)
}

// A Bash rule that names one of cadrei's own files is refused like an Edit
// rule would be (spec 3.5): the fixed Edit denies would mean little if a
// member's shell could change the same files. Commands on other folders,
// or on a folder that only holds a cadrei, stay allowed.
func TestBashRulesCannotReachCadreisOwnFiles(t *testing.T) {
	c, ph := layout(t, "plain")
	defer func(f bool) { foldCase = f }(foldCase)
	foldCase = true
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{
		"Bash(rm {CADREI}/members/dev/engineer.md)",
		"Bash(rm {CADREI}/personas/dev/engineer.md)",
		"Bash(cp x {CADREI}/members/dev/x.md)",
		"Bash(echo x > {CADREI}/members/dev/x.md)",
		"Bash(echo x >>{CADREI}/protocol.md)",
		"Bash(rm {CADREI}/playbook.md)",
		"Bash(tee {CADREI}/projects.yaml)",
		"Bash(rm -rf {CADREI}/.git)",
		"Bash(rm -rf {CADREI}/.claude/build)",
		"Bash(rm -rf {CADREI})",
		"Bash(rm ~/.cadrei/w/members/dev/engineer.md)",
		"Bash(rm ~/.cadrei/w/playbook.md)",
		"Bash(rm -rf ~/.cadrei/w)",
		"Bash(ls ~/.cadrei)",
		"Bash(cp x ~/.cadrei/config/default)",
		"Bash(rm -rf ~/.cadrei/framework)",
		"Bash(cp x $HOME/.cadrei/w/playbook.md)",
		"Bash(mv x {HOME}/Documents/link/protocol.md)",
		"Bash(cp --target-directory={CADREI}/members/dev x)",
		"Bash(dd if=x of=~/.cadrei/w/playbook.md)",
		"Bash(rm {OTHER}/members/x.md)",
		"Bash(rm {CADREI}/members/*)",
		// The variable every member has, pinned to its cadrei.
		"Bash(rm $CADREI_HOME/playbook.md)",
		"Bash(rm ${CADREI_HOME}/members/dev/engineer.md)",
		"Bash(rm -rf $CADREI_HOME)",
		// Any other variable could be a cadrei.
		"Bash(rm $X/playbook.md)",
		"Bash(rm ${WORK}/members/dev/x.md)",
		// The current user's home by name, and another spelling of the case.
		"Bash(rm ~" + me.Username + "/.cadrei/w/playbook.md)",
		"Bash(rm ~/.CADREI/W/PLAYBOOK.MD)",
		// Two or more .. straight to an own name: out of a team folder.
		"Bash(sed -i s/a/b/ ../../members/dev/x.md)",
		"Bash(rm ../../playbook.md)",
		"Bash(cat ../../members/list.json)",
		"Bash(rm ../../../w/../../playbook.md)",
		// The same climbs, not written in their simplest form.
		"Bash(rm ../../teams/../members/dev/engineer.md)",
		"Bash(rm ../.././members/dev/engineer.md)",
		"Bash(rm ..//../members/dev/engineer.md)",
		"Bash(rm ../../../w/members/dev/engineer.md)",
		"Bash(rm ../../../../.cadrei/w/playbook.md)",
		// Through a link (a, to the team folder) the kernel climbs twice.
		"Bash(rm a/../../members/dev/engineer.md)",
		"Bash(rm a/../../playbook.md)",
	} {
		if got, msg := check(c, "R", fill(rule, ph)); got != "refuse" || !strings.Contains(msg, "only the user changes") {
			t.Errorf("%s: %s %q", rule, got, mask(msg, ph))
		}
	}
	for _, rule := range []string{
		"Bash(ls ~)", "Bash(du -sh ~/work)", "Bash(npm test)", "Bash(cat {CADREI}/teams/dev/notes.md)",
		"Bash(rm {CADREI}/teams/dev/old.md)", "Bash(cp a ../b/c.md)", "Bash(ls ~/.cadrei-notes)", "Bash(cat members/dev/x.md)",
		// A project's own subfolders, one .. up.
		"Bash(cat ../.git/config)", "Bash(ls ../.claude)", "Bash(cat ../docs/playbook.md)",
		"Bash(cat src/../README.md)", "Bash(cat ../x/../.git/config)",
		"Bash(cat $PROJECT/src/main.go)", "Bash(echo $HOME)", "Bash(dd if=in.img of=out.img)",
	} {
		if got, msg := check(c, "R", fill(rule, ph)); got == "refuse" {
			t.Errorf("%s was refused: %q", rule, mask(msg, ph))
		}
	}
}
