// Package allow decides whether a grant may be added to the persona
// settings: it refuses rules that are too broad or that reach files which
// run code outside a persona's session or hold cadre's own state, warns
// about others, and checks plain-English --auto entries.
//
// It is a port of the bash 0.2.0 checker, which nine review rounds shaped.
// The messages are kept word for word: the skill and the tests match them.
package allow

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/shellwords"
)

// Refused is the reason a grant was refused, as cadre prints it.
type Refused string

func (r Refused) Error() string { return string(r) }

// Checker holds the folders a check needs, all physical.
type Checker struct {
	Root   string   // ~/.cadre, where cadres live (section N)
	Home   string   // the user's home folder
	Cache  string   // $XDG_CACHE_HOME, or ~/.cache
	Cadre  string   // the cadre whose personas get the grant
	Cadres []string // every known cadre, this one included
}

// New makes a Checker for cadre, with the user's home and cache folders
// from the environment. known lists the other known cadres.
func New(cadre string, known []string) *Checker {
	home := paths.Home()
	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		cache = home + "/.cache"
	}
	c := &Checker{Root: home + "/.cadre", Home: home, Cache: paths.Real(cache), Cadre: paths.Real(cadre)}
	c.Cadres = append([]string{c.Cadre}, known...)
	return c
}

// norm is text as it is matched: NFKC, no format characters, dashes and
// spaces made plain, case folded.
func normText(text string) string {
	var b strings.Builder
	for _, r := range norm.NFKC.String(text) {
		switch {
		case unicode.Is(unicode.Cf, r):
			continue
		case unicode.Is(unicode.Pd, r):
			b.WriteRune('-')
		case unicode.Is(unicode.Zs, r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return cases.Fold().String(b.String())
}

var ruleShape = regexp.MustCompile(`(?s)^([A-Za-z][A-Za-z0-9_*-]*)(?:\((.*)\))?$`)

// only reports whether every character of s is whitespace or in chars.
func only(s, chars string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) && !strings.ContainsRune(chars, r) {
			return false
		}
	}
	return true
}

var tldWildcard = regexp.MustCompile(`(?s)^\*\.[^.*]+$`)

// Rule checks a permission rule. It returns a warning to show (or "") when
// the rule may be added, and a Refused error when it may not.
func (c *Checker) Rule(rule string) (warning string, err error) {
	defer func() {
		if r := recover(); r != nil {
			ref, ok := r.(Refused)
			if !ok {
				panic(r)
			}
			warning, err = "", ref
		}
	}()
	return c.rule(rule), nil
}

// refuse stops a check with the reason; Rule and Auto turn it into an error.
func refuse(format string, a ...any) { panic(Refused(fmt.Sprintf(format, a...))) }

func (c *Checker) rule(rule string) string {
	if strings.TrimSpace(rule) == "*" {
		refuse("refused: * allows every tool; name one tool and a narrow use, such as Bash(npm test)")
	}
	m := ruleShape.FindStringSubmatchIndex(rule)
	if m == nil || strings.Contains(rule, "\n") {
		refuse("refused: '%s' is not a permission rule (Tool or Tool(specifier)); "+
			"for a plain-English allowance use: cadre allow add --auto \"<sentence>\"", rule)
	}
	tool := rule[m[2]:m[3]]
	hasSpec := m[4] >= 0
	spec := ""
	if hasSpec {
		spec = rule[m[4]:m[5]]
	}
	n := normText(rule)
	if strings.Contains(n, "persona-settings") || strings.Contains(n, "persona_settings") || cadreAllow.MatchString(n) {
		refuse("refused: %s targets the persona settings or cadre allow, which only the user changes", rule)
	}
	if strings.Contains(n, "cadre.conf") {
		refuse("refused: %s reaches cadre.conf, which cadre runs as shell code; make that edit yourself", rule)
	}
	if strings.Contains(tool, "*") {
		refuse("refused: %s puts a wildcard in the tool name; name one tool", rule)
	}
	for _, k := range known {
		if strings.EqualFold(tool, k) && tool != k {
			refuse("refused: write the tool name as %s", k)
		}
	}
	if strings.HasPrefix(tool, "mcp__") {
		parts := strings.Split(tool, "__")
		if len(parts) < 3 || parts[2] == "" {
			refuse("refused: %s allows a whole MCP server; name one tool, such as %s__<tool>", rule, tool)
		}
	}
	if !hasSpec {
		if bare[strings.ToLower(tool)] {
			refuse("refused: a bare %s allows every use of it; add a narrow specifier, such as %s(<exact command or path>)", tool, tool)
		}
		return ""
	}
	if tool == "Write" {
		refuse("refused: Claude Code does not use Write(path) rules; Edit(path) covers writing files")
	}
	if only(spec, "*:/~.") {
		refuse("refused: %s has only a wildcard; give an exact command, prefix or path", rule)
	}
	if tool == "WebFetch" && strings.HasPrefix(spec, "domain:") {
		host := strings.TrimSpace(spec[len("domain:"):])
		if only(host, "*.") || tldWildcard.MatchString(host) {
			refuse("refused: %s allows every domain or a whole top-level domain; name the domain, such as WebFetch(domain:docs.example.com)", rule)
		}
	}
	var warn string
	switch tool {
	case "Bash", "PowerShell":
		warn = c.command(rule, spec)
	case "Edit", "NotebookEdit", "Read":
		warn = c.path(rule, tool, spec)
	}
	if warn != "" {
		return warn
	}
	if strings.Contains(rule, "*") {
		return fmt.Sprintf("warning: %s contains * and may allow more than you mean", rule)
	}
	return ""
}

var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// program returns the program word of a command specifier and the words
// after it, after leading NAME=value words.
func program(spec string) (string, []string) {
	text := strings.ReplaceAll(spec, ":*", " *")
	words, err := shellwords.Split(text)
	if err != nil {
		words = strings.Fields(text)
	}
	for len(words) > 0 && assignment.MatchString(words[0]) && !strings.Contains(words[0], "\n") {
		words = words[1:]
	}
	if len(words) == 0 {
		return "", nil
	}
	return words[0], words[1:]
}

func (c *Checker) command(rule, spec string) string {
	if shellwords.HasOperator(strings.ReplaceAll(spec, ":*", " *")) {
		refuse("refused: %s chains or backgrounds commands (&&, ||, ;, | or &); allow each command on its own", rule)
	}
	prog, rest := program(spec)
	if prog == "" {
		refuse("refused: %s names no program; give the exact command", rule)
	}
	if strings.ContainsAny(prog, "$`()<>&;|") {
		refuse("refused: %s computes its program name with shell syntax; name the program exactly", rule)
	}
	if strings.ContainsAny(prog, "*?") {
		refuse("refused: %s has a wildcard in the program name, which matches other programs; name the program exactly", rule)
	}
	base := normText(path.Base(prog))
	base = strings.TrimSuffix(base, ".exe")
	wild := strings.Contains(spec, "*")
	if wild && (runners[base] || versioned.MatchString(base)) {
		refuse("refused: %s lets %s run anything; allow the exact command instead", rule, base)
	}
	if base == "cadre" && (len(rest) == 0 || strings.Contains(rest[0], "*") || normText(rest[0]) == "allow") {
		refuse("refused: %s would include cadre allow; name the cadre command, such as Bash(cadre up:*)", rule)
	}
	if base == "cadre" {
		switch normText(rest[0]) {
		case "use", "cadres", "init":
			refuse("refused: %s lets a persona register or switch cadres, which only the user does", rule)
		}
	}
	sub := ""
	for _, w := range rest {
		if !strings.HasPrefix(w, "-") {
			sub = normText(w)
			break
		}
	}
	if wild {
		for _, w := range rest {
			if subrunners[base][normText(w)] {
				refuse("refused: %s lets %s %s run anything; allow the exact command instead", rule, base, normText(w))
			}
		}
		if base == "find" {
			for _, w := range rest {
				switch w {
				case "-exec", "-execdir", "-ok", "-okdir":
					refuse("refused: %s lets find run any command; allow the exact command instead", rule)
				}
			}
		}
	}
	if base == "git" && wild {
		return fmt.Sprintf("warning: %s lets git run other programs (git -c, aliases); prefer exact git commands", rule)
	}
	if subs, ok := fromFiles[base]; wild && (strings.Contains(prog, "/") || (ok && (subs == nil || subs[sub]))) {
		return fmt.Sprintf("warning: %s runs code from files a persona can change (a Makefile, package.json, a script); prefer exact commands", rule)
	}
	return ""
}

// unescapeSlashes reads an escaped / as a plain /, reading escapes in
// pairs, so \\/ stays a backslash and then a separator.
func unescapeSlashes(spec string) string {
	rs := []rune(spec)
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		if rs[i] == '\\' && i+1 < len(rs) {
			if rs[i+1] == '/' {
				b.WriteRune('/')
			} else {
				b.WriteRune('\\')
				b.WriteRune(rs[i+1])
			}
			i++
			continue
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}

// fixedPart is a path up to its first wildcard, class, brace or escape.
func fixedPart(p string) string {
	if i := strings.IndexAny(p, `*?[{\`); i >= 0 {
		return p[:i]
	}
	return p
}

// path refuses Edit rules that reach protected or code-running files and
// cadre's own state, and warns on Read rules that reach secrets.
func (c *Checker) path(rule, tool, spec string) string {
	if trailing := len(spec) - len(strings.TrimRight(spec, `\`)); trailing%2 == 1 {
		refuse("refused: %s ends in a backslash; write the path without it", rule)
	}
	spec = unescapeSlashes(spec)
	rs := []rune(spec)
	for i := 1; i < len(rs); i++ {
		if rs[i-1] == '[' && rs[i] == ':' && (i < 2 || rs[i-2] != '\\') {
			refuse("refused: %s uses a [:class:] pattern; write the path literally", rule)
		}
	}
	var stack []int
	for i, ch := range rs {
		if (ch == '{' || ch == '}') && i > 0 && rs[i-1] == '\\' {
			continue
		}
		if ch == '{' {
			stack = append(stack, i)
		} else if ch == '}' && len(stack) > 0 {
			start := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(stack) > 0 || strings.ContainsRune(string(rs[start:i]), '/') {
				refuse("refused: %s nests braces or puts a / inside them; write each path as its own rule", rule)
			}
		}
	}
	for _, seg := range segments(spec) {
		if _, err := compile(segRegex(seg)); err != nil {
			refuse("refused: %s has a pattern cadre cannot read; write the path literally", rule)
		}
	}
	if strings.HasPrefix(spec, "/") && !strings.HasPrefix(spec, "//") {
		refuse("refused: %s starts with a single /, which Claude Code reads relative to the settings file; use //path for an absolute path", rule)
	}
	full := ""
	switch {
	case strings.HasPrefix(spec, "//"):
		full = "/" + strings.TrimLeft(spec[2:], "/")
		if strings.HasPrefix(cases.Fold().String(full), "/system/volumes/data/") {
			full = full[len("/System/Volumes/Data"):]
		}
	case spec == "~" || strings.HasPrefix(spec, "~/"):
		full = c.Home + spec[1:]
	}
	var forms []string
	if full != "" {
		// Check the path as written and with the folders before the first
		// wildcard resolved; refuse if either form reaches a target.
		head := fixedPart(full)
		head = head[:strings.LastIndex(head, "/")+1]
		if head == "" {
			head = "/"
		}
		forms = []string{full, strings.TrimRight(paths.Real(head), "/") + "/" + full[len(head):]}
	}
	var parts []string
	for _, p := range strings.Split(spec, "/") {
		parts = append(parts, normText(p))
	}
	for _, p := range parts {
		if spells(p, "..") {
			refuse("refused: %s has a .. segment; name the path without it", rule)
		}
	}
	hit := func(target string) bool {
		real := paths.Real(target)
		if strings.HasSuffix(target, "/") {
			real += "/"
		}
		for _, f := range forms {
			if reaches(f, target) || reaches(f, real) {
				return true
			}
		}
		return false
	}
	named := func(names ...string) bool {
		for i := 0; i+len(names) <= len(parts); i++ {
			all := true
			for k, x := range names {
				if !spells(parts[i+k], x) {
					all = false
					break
				}
			}
			if all {
				return true
			}
		}
		return false
	}
	if tool != "Read" {
		for _, suffix := range outsideSuffixes {
			if named(strings.Split(suffix, "/")...) {
				refuse("refused: %s reaches %s, which runs code outside a persona's session or holds cadre's own state; make that edit yourself", rule, suffix)
			}
		}
	}
	if tool == "Read" {
		for _, t := range []string{c.Home + "/.ssh/", c.Home + "/.aws/", c.Home + "/.gnupg/", c.Home + "/.config/gh/"} {
			if hit(t) {
				return fmt.Sprintf("warning: %s lets personas read secrets (~/.ssh, ~/.aws, ~/.gnupg, ~/.config/gh); keep it as narrow as you can", rule)
			}
		}
		return ""
	}
	for _, name := range protected {
		if named(name) || hit(c.Home+"/"+name) {
			refuse("refused: Claude Code never pre-approves writes to %s; make that edit yourself", name)
		}
	}
	if named(".claude") || hit(c.Home+"/.claude/") || hit(c.Home+"/.config/git/") {
		refuse("refused: Claude Code never pre-approves writes under .claude or .config/git; make that edit yourself")
	}
	// A relative rule is read from the persona's folder, which may be the cadre itself.
	if named("cadre.conf") || (full == "" && reaches(spec, "cadre.conf")) {
		refuse("refused: %s reaches %s/cadre.conf, which cadre runs as shell code; make that edit yourself", rule, c.Cadre)
	}
	for _, cadre := range c.Cadres {
		if hit(cadre + "/cadre.conf") {
			refuse("refused: %s reaches %s/cadre.conf, which cadre runs as shell code; make that edit yourself", rule, cadre)
		}
	}
	for _, f := range forms {
		fixed := strings.TrimRight(fixedPart(f), "/")
		if fixed == "" || c.Home == fixed || strings.HasPrefix(c.Home, fixed+"/") {
			refuse("refused: %s covers your whole home folder; name the folder or file", rule)
		}
	}
	for _, t := range c.outside() {
		if hit(t) {
			shown := strings.ReplaceAll(strings.TrimSuffix(t, "/"), anyName, "<cadre>")
			refuse("refused: %s lets a persona change %s, which runs code outside its session or holds cadre's own state; make that edit yourself", rule, shown)
		}
	}
	for _, x := range outsideNames {
		if named(x) {
			refuse("refused: %s lets a persona change files that run code outside its session; make that edit yourself", rule)
		}
	}
	for _, cadre := range c.Cadres {
		if cadre != c.Cadre && hit(cadre+"/teams/") {
			return fmt.Sprintf("warning: %s reaches the team folders of the cadre at %s; this cadre's personas would change that cadre's work", rule, cadre)
		}
	}
	return ""
}

// outside lists files and folders (ending in /) that run code outside a
// persona's session, and cadre's own state.
func (c *Checker) outside() []string {
	h := c.Home
	out := []string{h + "/.ssh/", h + "/.config/fish/", h + "/.tmux.conf", h + "/.vimrc",
		h + "/Library/LaunchAgents/", h + "/.config/autostart/", h + "/.config/systemd/user/",
		h + "/.local/bin/", h + "/.config/cadre/", c.Cache + "/cadre/",
		"/var/spool/cron/", "/usr/lib/cron/tabs/", "/etc/crontab"}
	cadres := c.Cadres
	if c.Root != "" {
		// Cadre's own files under ~/.cadre (N.7), for any cadre there, also
		// ones made later.
		any := c.Root + "/" + anyName
		out = append(out, c.Root+"/config/", c.Root+"/framework/", any+"/cadre.conf")
		cadres = append(append([]string{}, cadres...), any)
	}
	for _, cadre := range cadres {
		// A cadre's own files: everything but its team folders.
		out = append(out, cadre+"/.claude/build/", cadre+"/.claude/", cadre+"/personas/", cadre+"/playbook.md",
			cadre+"/protocol.md", cadre+"/projects.yaml", cadre+"/.git/")
	}
	return out
}

// Auto checks a plain-English --auto entry. It returns a warning to show
// (or "") when it may be added, and a Refused error when it may not.
func (c *Checker) Auto(text string) (warning string, err error) {
	defer func() {
		if r := recover(); r != nil {
			ref, ok := r.(Refused)
			if !ok {
				panic(r)
			}
			warning, err = "", ref
		}
	}()
	return auto(text), nil
}

func auto(text string) string {
	if strings.Contains(text, "\n") || strings.TrimSpace(text) == "" {
		refuse("refused: an --auto entry is one non-empty line")
	}
	if n := utf8.RuneCountInString(text); n > 300 {
		refuse("refused: an --auto entry is at most 300 characters (this one has %d)", n)
	}
	for _, r := range text {
		if r > 127 && unicode.IsLetter(r) {
			refuse("refused: write --auto entries in plain ASCII letters")
		}
	}
	n := normText(text)
	if strings.TrimSpace(n) == "$defaults" {
		refuse("refused: $defaults is already there")
	}
	if autoRefused.MatchString(n) || strings.Contains(n, "persona-settings") {
		refuse("refused: an --auto entry may not speak about permissions, settings, grants, cadre allow " +
			"or the user's approval; describe the work that is expected instead")
	}
	for _, w := range blanket {
		if strings.Contains(n, w) {
			return fmt.Sprintf("warning: the entry says \"%s\"; check that it does not allow more than you mean", w)
		}
	}
	return ""
}
