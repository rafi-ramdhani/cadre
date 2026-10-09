// Package backup keeps credentials and bulky files out of a cadre's
// backups: the pre-push hook cadre installs in each cadre repository, and
// the check it runs (cadre hook pre-push).
package backup

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// marker is the line that makes a pre-push hook cadre's own.
const marker = "# cadre: refuses to push credentials and large files (cadre hook pre-push)"

// MaxSize is the largest file a push may carry (a variable for tests).
var MaxSize int64 = 50 << 20

// scanSize is the largest file whose content is searched for tokens.
const scanSize = 8 << 20

// hookFile is the cadre repository's pre-push hook, as git finds it
// (core.hooksPath included).
func hookFile(repo string) (string, error) {
	out, err := exec.Command("git", "-C", repo, "rev-parse", "--git-path", "hooks/pre-push").Output()
	if err != nil {
		return "", fmt.Errorf("%s is not a git repository", repo)
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(repo, p)
	}
	return p, nil
}

// hookText is the hook that runs binary.
func hookText(binary string) string {
	return "#!/bin/sh\n" + marker + "\nexec '" + strings.ReplaceAll(binary, "'", `'\''`) + "' hook pre-push \"$@\"\n"
}

// ErrForeign is returned when the repository has a pre-push hook that is
// not cadre's: cadre leaves it alone.
var ErrForeign = errors.New("has a pre-push hook that is not cadre's")

// Install makes cadre's pre-push hook run binary in the cadre repository
// repo. It reports whether it wrote anything. A hook that is not cadre's is
// left as it is, with ErrForeign.
func Install(repo, binary string) (bool, error) {
	file, err := hookFile(repo)
	if err != nil {
		return false, err
	}
	want := hookText(binary)
	raw, err := os.ReadFile(file)
	switch {
	case err == nil && string(raw) == want:
		return false, nil
	case err == nil && !bytes.Contains(raw, []byte(marker)):
		return false, fmt.Errorf("%s %w", repo, ErrForeign)
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return false, err
	}
	tmp := file + ".cadre-new"
	if err := os.WriteFile(tmp, []byte(want), 0o755); err != nil {
		return false, err
	}
	return true, os.Rename(tmp, file)
}

// A Finding is a file a push would carry that it must not.
type Finding struct {
	Path string
	Why  string
}

var (
	// Files that hold credentials by their name.
	secretNames = map[string]string{
		".credentials.json": "a credentials file",
		".netrc":            "a credentials file",
		".npmrc":            "may hold a registry token",
		".pypirc":           "may hold a registry token",
		"id_rsa":            "a private key",
		"id_ecdsa":          "a private key",
		"id_ed25519":        "a private key",
		"id_dsa":            "a private key",
	}
	secretExts = map[string]string{".pem": "a key or certificate", ".key": "a private key",
		".p12": "a key store", ".pfx": "a key store"}
	// Token patterns in a file's content.
	tokens = []struct {
		re   *regexp.Regexp
		what string
	}{
		{regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{16,}`), "an Anthropic API key"},
		{regexp.MustCompile(`ghp_[A-Za-z0-9]{30,}`), "a GitHub token"},
		{regexp.MustCompile(`github_pat_[A-Za-z0-9_]{30,}`), "a GitHub token"},
		{regexp.MustCompile(`gh[ousr]_[A-Za-z0-9]{30,}`), "a GitHub token"},
		{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`), "a private key"},
	}
)

// nameWhy says why a file's name marks it as a credential, or "".
func nameWhy(p string) string {
	base := path.Base(p)
	if why, ok := secretNames[base]; ok {
		return "looks like " + why
	}
	if base == ".env" || (strings.HasPrefix(base, ".env.") && !envExample(base)) {
		return "looks like an environment file with secrets"
	}
	if why, ok := secretExts[strings.ToLower(path.Ext(base))]; ok {
		return "looks like " + why
	}
	return ""
}

// envExample reports whether a .env.* file is a template meant to be shared.
func envExample(base string) bool {
	switch strings.TrimPrefix(base, ".env.") {
	case "example", "sample", "template", "dist":
		return true
	}
	return false
}

// contentWhy says which token a file's content holds, or "".
func contentWhy(data []byte) string {
	for _, t := range tokens {
		if t.re.Match(data) {
			return "holds what looks like " + t.what
		}
	}
	return ""
}

// Update is one ref a push updates, as git gives it on the hook's stdin.
type Update struct{ LocalRef, LocalSHA, RemoteRef, RemoteSHA string }

// ReadUpdates parses the pre-push hook's stdin.
func ReadUpdates(r io.Reader) []Update {
	var out []Update
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 4 {
			out = append(out, Update{f[0], f[1], f[2], f[3]})
		}
	}
	return out
}

func deleted(sha string) bool { return strings.Trim(sha, "0") == "" }

// Scan checks every file the push carries that no remote has yet: in every
// commit pushed, not only the last, since history goes with it.
func Scan(repo string, updates []Update) ([]Finding, error) {
	paths := map[string][]string{} // blob -> paths
	for _, u := range updates {
		if deleted(u.LocalSHA) {
			continue
		}
		out, err := exec.Command("git", "-C", repo, "rev-list", "--objects", u.LocalSHA, "--not", "--remotes").Output()
		if err != nil {
			return nil, fmt.Errorf("could not list what the push carries: %v", err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			sha, p, ok := strings.Cut(line, " ")
			if ok && p != "" {
				paths[sha] = append(paths[sha], p)
			}
		}
	}
	if len(paths) == 0 {
		return nil, nil
	}
	shas := make([]string, 0, len(paths))
	for sha := range paths {
		shas = append(shas, sha)
	}
	sort.Strings(shas)
	var findings []Finding
	add := func(sha, why string) {
		for _, p := range paths[sha] {
			findings = append(findings, Finding{p, why})
		}
	}
	// One git process gives each object's type, size and, when small
	// enough, its content.
	cmd := exec.Command("git", "-C", repo, "cat-file", "--batch")
	in, _ := cmd.StdinPipe()
	outPipe, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		w := bufio.NewWriter(in)
		for _, sha := range shas {
			fmt.Fprintln(w, sha)
		}
		w.Flush()
		in.Close()
	}()
	r := bufio.NewReader(outPipe)
	for _, sha := range shas {
		header, err := r.ReadString('\n')
		if err != nil {
			cmd.Wait()
			return nil, fmt.Errorf("could not read the pushed files: %v", err)
		}
		f := strings.Fields(header)
		if len(f) != 3 {
			continue // missing object
		}
		size, _ := strconv.ParseInt(f[2], 10, 64)
		var data []byte
		if f[1] == "blob" && size <= scanSize {
			data = make([]byte, size)
			if _, err := io.ReadFull(r, data); err != nil {
				cmd.Wait()
				return nil, err
			}
		} else if _, err := io.CopyN(io.Discard, r, size); err != nil {
			cmd.Wait()
			return nil, err
		}
		r.ReadByte() // the newline after the content
		if f[1] != "blob" {
			continue
		}
		why := ""
		for _, p := range paths[sha] {
			if w := nameWhy(p); w != "" {
				findings = append(findings, Finding{p, w})
				why = w
			}
		}
		if why != "" {
			continue
		}
		switch {
		case size > MaxSize:
			add(sha, fmt.Sprintf("is %d MB, over the %d MB limit", size>>20, MaxSize>>20))
		case data != nil:
			if w := contentWhy(data); w != "" {
				add(sha, w)
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("could not read the pushed files: %v", err)
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Path < findings[j].Path })
	return findings, nil
}

// Remove takes out cadre's pre-push hook from a cadre repository: only a
// hook cadre wrote (its marker line), never one of the user's. It reports
// whether it removed one.
func Remove(repo string) (bool, error) {
	file, err := hookFile(repo)
	if err != nil {
		return false, nil
	}
	raw, err := os.ReadFile(file)
	if err != nil || !bytes.Contains(raw, []byte(marker)) {
		return false, nil
	}
	return true, os.Remove(file)
}

// Installed reports whether a cadre repository has cadre's pre-push hook.
func Installed(repo string) bool {
	file, err := hookFile(repo)
	if err != nil {
		return false
	}
	raw, err := os.ReadFile(file)
	return err == nil && bytes.Contains(raw, []byte(marker))
}
