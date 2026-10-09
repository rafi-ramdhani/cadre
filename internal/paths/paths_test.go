package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// tempDir is t.TempDir in its physical form (macOS: /var is /private/var).
func tempDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestReal(t *testing.T) {
	d := tempDir(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(d, "real", "sub"), 0o755))
	must(os.Symlink(filepath.Join(d, "real"), filepath.Join(d, "abs")))
	must(os.Symlink("real/sub", filepath.Join(d, "rel")))
	must(os.Symlink("abs", filepath.Join(d, "chain")))
	must(os.Symlink("loop", filepath.Join(d, "loop")))
	for _, tc := range []struct{ in, want string }{
		{d + "/abs/sub", d + "/real/sub"},
		{d + "/rel", d + "/real/sub"},
		{d + "/chain/sub/missing/deeper", d + "/real/sub/missing/deeper"},
		{d + "/rel/../x", d + "/real/x"},       // .. after a link is physical
		{d + "/missing/../real", d + "/real"},  // .. after a missing part
		{d + "//real/./sub/", d + "/real/sub"}, // doubled and dot parts
		{d + "/loop/x", d + "/loop/x"},         // a loop ends
		{"/", "/"},
	} {
		if got := Real(tc.in); got != tc.want {
			t.Errorf("Real(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRealRelative(t *testing.T) {
	d := tempDir(t)
	t.Chdir(d)
	if got := Real("x/y"); got != d+"/x/y" {
		t.Errorf("Real(x/y) = %q", got)
	}
}

func TestHomeUsesHOME(t *testing.T) {
	d := tempDir(t)
	t.Setenv("HOME", d)
	if Home() != d {
		t.Errorf("Home() = %q, want %q", Home(), d)
	}
}

func TestWithin(t *testing.T) {
	for _, tc := range []struct {
		p, dir string
		want   bool
	}{
		{"/a/b", "/a", true}, {"/a", "/a", true}, {"/ab", "/a", false}, {"/a/b", "/a/", true}, {"/x", "/", true},
	} {
		if Within(tc.p, tc.dir) != tc.want {
			t.Errorf("Within(%q, %q) != %v", tc.p, tc.dir, tc.want)
		}
	}
}
