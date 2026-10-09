package cadres

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rafi-ramdhani/cadre/internal/paths"
)

// Create makes a new cadre in ~/.cadre/<name> from the template (fs holds
// template/ at its root, as the embedded assets do). The cadre is a git
// repository; its first commit is made when git has an identity, and note
// says so otherwise.
func Create(name string, tmpl fs.FS) (c Cadre, note string, err error) {
	if err := CheckName(name); err != nil {
		return c, "", err
	}
	dest := filepath.Join(Root(), name)
	if _, err := os.Lstat(dest); err == nil {
		return c, "", fmt.Errorf("%s already exists", dest)
	}
	if other, ok := Clash(name, paths.Real(dest)); ok {
		return c, "", fmt.Errorf("a cadre named %s is already at %s; choose another name", other.Name, other.Path)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return c, "", err
	}
	if err := writeTemplate(tmpl, dest, name); err != nil {
		return c, "", err
	}
	c = Cadre{Name: name, Path: paths.Real(dest)}
	if out, err := git(c.Path, "init", "-q", "-b", "main"); err != nil {
		return c, "", fmt.Errorf("git init in %s: %s", c.Path, out)
	}
	note, err = Commit(c.Path, "Start "+name+" from the cadre template", ".")
	return c, note, err
}

// writeTemplate copies template/ into dest, putting the cadre's name into
// its Markdown files. A cadre has no projects/ folder, so the .gitignore
// lines for it are left out.
func writeTemplate(tmpl fs.FS, dest, name string) error {
	return fs.WalkDir(tmpl, "template", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("template", p)
		to := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		data, err := fs.ReadFile(tmpl, p)
		if err != nil {
			return err
		}
		if strings.HasSuffix(p, ".md") {
			data = bytes.ReplaceAll(data, []byte("{{name}}"), []byte(name))
		}
		if rel == ".gitignore" {
			data = withoutProjects(data)
		}
		return os.WriteFile(to, data, 0o644)
	})
}

// withoutProjects drops the projects/ lines of the template's .gitignore,
// and the comment that explains them.
func withoutProjects(data []byte) []byte {
	var kept []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.Contains(l, "/projects/") || strings.Contains(l, "Project repos live here") {
			continue
		}
		kept = append(kept, l)
	}
	return []byte(strings.Join(kept, "\n"))
}

func git(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Commit commits files (paths inside the cadre) with msg. Without a git
// identity it leaves the change uncommitted and returns a note saying so;
// a cadre that is not a git repository is left alone.
func Commit(cadre, msg string, files ...string) (string, error) {
	if _, err := git(cadre, "rev-parse", "--git-dir"); err != nil {
		return "", nil
	}
	if _, err := git(cadre, "config", "user.email"); err != nil {
		return "note: git has no user identity, so this change was left uncommitted in " + cadre +
			"; set one (git config --global user.name/user.email) and commit", nil
	}
	if out, err := git(cadre, append([]string{"add", "--"}, files...)...); err != nil {
		return "", fmt.Errorf("git add: %s", out)
	}
	// Nothing to commit is not an error, as in the bash version.
	git(cadre, append([]string{"commit", "-qm", msg, "--"}, files...)...)
	return "", nil
}
