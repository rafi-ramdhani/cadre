package project

import "testing"

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
