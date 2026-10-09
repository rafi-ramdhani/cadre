package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `# One entry per project.
# Projects are their own git repos.

cadrei:
  repo: rafi-ramdhani/cadrei
  team: dev
  about: the framework

# My app, kept elsewhere
app:
  repo: me/app
  team: dev
  about: the app: web and api
  path: ~/Developer/app
`

func TestParse(t *testing.T) {
	f := Parse(sample)
	if len(f.Entries()) != 2 || f.Entries()[0].Name != "cadrei" || f.Entries()[1].Name != "app" {
		t.Fatalf("entries %+v", f.Entries())
	}
	app := f.Get("app")
	if app.Get("about") != "the app: web and api" || app.Get("path") != "~/Developer/app" || app.Get("missing") != "" {
		t.Errorf("app fields %v", app.Fields)
	}
	if f.Get("nope") != nil {
		t.Error("found an entry that is not there")
	}
	if string(f.Bytes()) != sample {
		t.Error("an unchanged registry is not written back as it was")
	}
}

func TestParseOddLines(t *testing.T) {
	f := Parse("a:\n  repo: x\nb:\n\trepo: y\n   # comment: not a field\nrepo: z\na:\n  team: t\n")
	if len(f.Entries()) != 2 || f.Entries()[0].Name != "a" {
		t.Fatalf("entries %+v", f.Entries())
	}
	if f.Get("b").Get("repo") != "z" || f.Get("a").Get("repo") != "" || f.Get("a").Get("team") != "t" {
		t.Errorf("fields: a %v, b %v", f.Get("a").Fields, f.Get("b").Fields)
	}
}

func TestEdits(t *testing.T) {
	f := Parse(sample)
	f.Add("blog", Field{"repo", "me/blog"}, Field{"team", "research"}, Field{"about", ""})
	if !f.Set("app", "path", "/Volumes/x/app") || !f.Set("cadrei", "path", "~/x") || f.Set("nope", "path", "x") {
		t.Fatal("Set results")
	}
	if !f.Remove("cadrei") || f.Remove("cadrei") {
		t.Fatal("Remove results")
	}
	want := `# One entry per project.
# Projects are their own git repos.

# My app, kept elsewhere
app:
  repo: me/app
  team: dev
  about: the app: web and api
  path: /Volumes/x/app

blog:
  repo: me/blog
  team: research
  about:
`
	if got := string(f.Bytes()); got != want {
		t.Errorf("after edits:\n%s\nwant:\n%s", got, want)
	}
	p := filepath.Join(t.TempDir(), "projects.yaml")
	if err := f.Save(p); err != nil {
		t.Fatal(err)
	}
	g, err := Load(p)
	if err != nil || g.Get("blog").Get("team") != "research" {
		t.Errorf("reload: %v", err)
	}
	if e, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err != nil || len(e.Entries()) != 0 {
		t.Errorf("a missing registry: %v %v", e, err)
	}
	if b, _ := os.ReadFile(p); !strings.HasSuffix(string(b), "about:\n") {
		t.Errorf("an empty value keeps a trailing space: %q", b)
	}
}

func TestSetAddsAFieldAfterTheLastOne(t *testing.T) {
	f := Parse("x:\n  repo: r\n# note\ny:\n  repo: s\n")
	f.Set("x", "path", "/p")
	if got := string(f.Bytes()); got != "x:\n  repo: r\n  path: /p\n# note\ny:\n  repo: s\n" {
		t.Errorf("got %q", got)
	}
	f = Parse("bare:\n")
	f.Set("bare", "path", "/p")
	if got := string(f.Bytes()); got != "bare:\n  path: /p\n" {
		t.Errorf("got %q", got)
	}
}

// Python read Unicode spaces as whitespace and a lone CR as a line end.
func TestParseExoticWhitespace(t *testing.T) {
	f := Parse("a:\r  repo: x\r\n\u00a0team: t\n\u3000# c\nb:\u00a0\n")
	if f.Get("a") == nil || f.Get("a").Get("repo") != "x" || f.Get("a").Get("team") != "t" || f.Get("b") == nil {
		t.Errorf("entries %+v", f.Entries())
	}
}

// A name that is not a project name never reaches a caller, so no path is
// built from it; the file itself keeps it, as the user wrote it.
func TestBadNamesAreLeftOut(t *testing.T) {
	text := "../.vim/pack/x/start/evil:\n  repo: r\na/b:\n  repo: r\n/abs:\n  repo: r\n.hidden:\n  repo: r\n-x:\n  repo: r\nx..y:\n  repo: r\nok.app:\n  repo: r\n"
	f := Parse(text)
	if e := f.Entries(); len(e) != 1 || e[0].Name != "ok.app" {
		t.Errorf("entries %+v", e)
	}
	if got := strings.Join(f.Skipped(), " "); got != "../.vim/pack/x/start/evil a/b /abs .hidden -x x..y" {
		t.Errorf("skipped %q", got)
	}
	if f.Get("../.vim/pack/x/start/evil") != nil || f.Get("a/b") != nil || f.Get("ok.app") == nil {
		t.Error("Get returned a bad name")
	}
	if string(f.Bytes()) != text {
		t.Error("the file was changed")
	}
}
