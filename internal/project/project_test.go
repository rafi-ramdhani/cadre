package project

import (
	"os"
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
