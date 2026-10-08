package allow

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// layout builds the fake HOME the golden outcomes were recorded in (see
// testdata/golden.tsv): a cadre named demo, a second known cadre, a symlink
// into the cadre, and for "stow", ~/.config and ~/.ssh as symlinks into
// ~/dotfiles.
func layout(t *testing.T, kind string) (*Checker, map[string]string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := root + "/home"
	cadre := home + "/work/demo"
	other := root + "/multi/b"
	for _, d := range []string{cadre + "/.claude", cadre + "/personas", other + "/personas", home + "/Documents"} {
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
	os.Symlink(cadre, home+"/Documents/link")
	c := &Checker{Root: home + "/.cadre", Home: home, Cache: home + "/.cache", Cadre: cadre, Cadres: []string{cadre, cadre, other}}
	return c, map[string]string{"{CADRE}": cadre, "{OTHER}": other, "{PARENT}": home + "/work", "{HOME}": home, "{ROOT}": root}
}

// fill puts the layout's paths into a placeholder text, and mask takes
// them out again, longest first.
func fill(s string, ph map[string]string) string {
	for _, k := range []string{"{CADRE}", "{OTHER}", "{PARENT}", "{HOME}", "{ROOT}"} {
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
		"CADRE":          "cadre",
		"c\u200badre":    "cadre", // a zero-width space (Cf) is dropped
		"persona\u2011x": "persona-x",
		"a\u00a0b":       "a b",
		"ｃａｄｒｅ":          "cadre", // fullwidth letters fold under NFKC
		"Straße":         "strasse",
		"ﬁle":            "file",
		"\u2014":         "-",
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
	if c.Home != home || c.Cache != home+"/.cache" || c.Cadre != home+"/work/demo" || len(c.Cadres) != 2 {
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
	c := &Checker{Home: home, Cache: home + "/.cache", Cadre: home + "/work/demo", Cadres: []string{home + "/work/demo"}}
	f.Fuzz(func(t *testing.T, rule string) {
		c.Rule(rule)
		c.Auto(rule)
	})
}
