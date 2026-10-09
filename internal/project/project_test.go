package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnerRepo(t *testing.T) {
	for _, in := range []string{"me/app", "Me/App", "https://github.com/me/app", "https://github.com/me/app.git",
		"git@github.com:me/app.git", "ssh://git@github.com/me/app", "https://github.com/me/app/"} {
		if got := OwnerRepo(in); got != "me/app" {
			t.Errorf("OwnerRepo(%q) = %q", in, got)
		}
	}
	if OwnerRepo("me/app") == OwnerRepo("you/app") {
		t.Error("different owners compare equal")
	}
}

func TestCheckName(t *testing.T) {
	for _, ok := range []string{"app", "my.app", "a_b-c"} {
		if CheckName(ok) != nil {
			t.Errorf("%s refused", ok)
		}
	}
	for _, bad := range []string{"", "..", "a..b", "../x", ".hidden", "a/b", "a b"} {
		if CheckName(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSameRepo(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"me/app", "https://github.com/me/app.git", true},
		{"git@github.com:me/app.git", "https://GitHub.com/me/app", true},
		{"https://github.com/me/app", "https://gitlab.com/me/app", false},
		{"git@gitlab.com:me/app.git", "https://github.com/me/app", false},
		{"ssh://git@github.com:22/me/app", "https://github.com/me/app", true},
		{"me/app", "you/app", false},
		{"", "", false},
		{"github.com/me/app", "me/app", true},
		{"github.com/me/app", "git@github.com:me/app.git", true},
		// Another host that only spells github.com in its path or name.
		{"github.com/acme/tool", "https://evil.example/github.com/acme/tool", false},
		{"github.com/acme/tool", "https://github.com.evil.example/acme/tool", false},
		{"acme/tool", "https://evil.example/acme/tool", false},
		{"github.com/acme/tool", "https://github.com/acme/tool.evil", false},
		// A repository on disk matches only itself.
		{"github.com/acme/tool", "file:///x/acme/tool", false},
		{"acme/tool", "/x/acme/tool", false},
		{"/x/acme/tool.git", "/x/acme/tool.git", true},
		{"file:///x/acme/tool", "/x/acme/tool", true},
		{"/x/acme/tool", "/y/acme/tool", false},
	} {
		if got := SameRepo(tc.a, tc.b); got != tc.want {
			t.Errorf("SameRepo(%q, %q) = %v", tc.a, tc.b, got)
		}
	}
}

func TestCloneRefusesOptions(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	for _, repo := range []string{"--upload-pack=touch " + marker, "-u touch " + marker} {
		if err := Clone(repo, filepath.Join(dir, "x")); err == nil || !strings.Contains(err.Error(), "cannot start with -") {
			t.Errorf("Clone(%q): %v", repo, err)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("an option in a repo ran")
	}
}

// Finder reads the projects folder's origins once and finds a clone of a
// repository by any spelling of it.
func TestFinder(t *testing.T) {
	dir := t.TempDir()
	for name, origin := range map[string]string{"app": "https://github.com/me/app.git", "blog": "git@github.com:me/blog.git", "plain": ""} {
		p := filepath.Join(dir, name)
		os.MkdirAll(p, 0o755)
		exec.Command("git", "-C", p, "init", "-q").Run()
		if origin != "" {
			exec.Command("git", "-C", p, "remote", "add", "origin", origin).Run()
		}
	}
	f := NewFinder(dir)
	if f.Find("me/app") != filepath.Join(dir, "app") || f.Find("github.com/me/blog") != filepath.Join(dir, "blog") || f.Find("me/other") != "" || f.Find("") != "" {
		t.Error("Find")
	}
	// The origins were read once: a clone added later is not looked at.
	os.MkdirAll(filepath.Join(dir, "late"), 0o755)
	exec.Command("git", "-C", filepath.Join(dir, "late"), "init", "-q").Run()
	exec.Command("git", "-C", filepath.Join(dir, "late"), "remote", "add", "origin", "https://github.com/me/late").Run()
	if f.Find("me/late") != "" {
		t.Error("the projects folder was read again")
	}
	if NewFinder("").Find("me/app") != "" {
		t.Error("no projects folder finds nothing")
	}
}
