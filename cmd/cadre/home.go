//go:build !windows

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/term"

	cadre "github.com/rafi-ramdhani/cadre"
	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/registry"
)

// interactive reports whether cadre may ask the user: stdin is a terminal,
// or, in a test build, answers come from stdin (see tty*.go).
func (e *env) interactive() bool {
	if testTTY() {
		return true
	}
	f, ok := e.stdin.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// ask prints question on stderr and reads one line of answer.
func (e *env) ask(question string) string {
	fmt.Fprint(e.stderr, question)
	line, _ := bufio.NewReader(e.stdin).ReadString('\n')
	return strings.TrimSpace(line)
}

// home prepares ~/.cadre: the one-time copy of ~/.config/cadre (N.1).
func (e *env) home() bool {
	msg, err := cadres.CopyOldConfig()
	if err != nil {
		e.fail("%s", err)
		return false
	}
	if msg != "" {
		fmt.Fprintln(e.stderr, "note: "+msg)
	}
	return true
}

// resolve finds the cadre this command acts on (N.3), asking which one
// when several cadres link the project the user is in.
func (e *env) resolve() (*cadres.Resolved, bool) {
	if !e.home() {
		return nil, false
	}
	var ask cadres.Asker
	if e.interactive() {
		ask = func(project string, names []string) (string, error) {
			return e.ask(fmt.Sprintf("%s is linked by %s. Which cadre? [%s] ", project, strings.Join(names, " and "), strings.Join(names, "/"))), nil
		}
	}
	wd, err := paths.Getwd()
	if err != nil {
		wd = paths.Home()
	}
	r, err := cadres.Resolve(wd, ask)
	if err != nil {
		e.fail("%s", err)
		return nil, false
	}
	return r, true
}

// persona refuses a command in a persona session.
func (e *env) persona(why string) bool {
	if os.Getenv("CADRE_PERSONA") != "" {
		e.fail("persona sessions cannot %s; ask the user in the orchestrator", why)
		return true
	}
	return false
}

func runInit(e *env) int {
	if len(e.args) < 1 || len(e.args) > 2 || strings.HasPrefix(e.args[0], "-") {
		return e.fail("usage: cadre init <name>")
	}
	if e.persona("register or switch cadres") || !e.home() {
		return 1
	}
	parent := ""
	if len(e.args) == 2 {
		parent = e.args[1] // the 0.1.x form: a visible cadre at <dir>/<name>
	}
	c, note, err := cadres.Create(e.args[0], parent, cadre.Assets)
	if err != nil {
		return e.fail("%s", err)
	}
	e.say("created %s", c.Path)
	if note != "" {
		e.say("%s", note)
	}
	def := cadres.Default()
	if d, ok := cadres.Find(def); def == "" || !ok || !d.Present() {
		if err := cadres.SetDefault(c.Name); err != nil {
			return e.fail("%s", err)
		}
		e.say("%s is the default cadre", c.Name)
	} else {
		e.say("default cadre stays %s; run cadre use %s to change it, or work in one of its projects", def, c.Name)
	}
	return 0
}

// isDirArg reports whether a use or cadres argument names a folder rather
// than a cadre.
func isDirArg(arg string) bool {
	if strings.ContainsRune(arg, '/') || arg == "." || arg == ".." || strings.HasPrefix(arg, "~") {
		return true
	}
	st, err := os.Stat(arg)
	return err == nil && st.IsDir()
}

// outsideCadre checks a folder to list as an outside cadre and returns it.
func outsideCadre(dir string) (cadres.Cadre, error) {
	p := paths.Real(dir)
	c := cadres.Cadre{Name: filepath.Base(p), Path: p, External: !cadres.Inside(p)}
	if !c.Present() {
		return c, fmt.Errorf("%s is not a cadre (no personas/ folder)", dir)
	}
	if err := cadres.CheckName(c.Name); err != nil {
		return c, err
	}
	if other, ok := cadres.Clash(c.Name, p); ok {
		return c, fmt.Errorf("a cadre named %s is already at %s; rename one of the folders", other.Name, other.Path)
	}
	return c, nil
}

func runUse(e *env) int {
	if len(e.args) != 1 {
		return e.fail("usage: cadre use <name>")
	}
	if e.persona("register or switch cadres") || !e.home() {
		return 1
	}
	var c cadres.Cadre
	if isDirArg(e.args[0]) {
		var err error
		if c, err = outsideCadre(e.args[0]); err != nil {
			return e.fail("%s", err)
		}
		if c.External {
			if err := cadres.AddExternal(c.Path); err != nil {
				return e.fail("%s", err)
			}
		}
	} else {
		var ok bool
		if c, ok = cadres.Find(e.args[0]); !ok || !c.Present() {
			return e.fail("no cadre named %s (see cadre ls --all)", e.args[0])
		}
	}
	if err := cadres.SetDefault(c.Name); err != nil {
		return e.fail("%s", err)
	}
	e.say("default cadre: %s (%s)", c.Name, c.Path)
	return 0
}

func runCadresAdd(e *env) int {
	if len(e.args) != 1 {
		return e.fail("usage: cadre cadres add <dir>")
	}
	if e.persona("register or switch cadres") || !e.home() {
		return 1
	}
	c, err := outsideCadre(e.args[0])
	if err != nil {
		return e.fail("%s", err)
	}
	if !c.External {
		return e.fail("%s is in ~/.cadre, where every cadre is found without adding it", c.Path)
	}
	if err := cadres.AddExternal(c.Path); err != nil {
		return e.fail("%s", err)
	}
	e.say("added cadre %s (%s)", c.Name, c.Path)
	return 0
}

func runCadresRemove(e *env) int {
	if len(e.args) != 1 {
		return e.fail("usage: cadre cadres remove <name|dir>")
	}
	if e.persona("register or switch cadres") || !e.home() {
		return 1
	}
	arg := e.args[0]
	list, _ := cadres.List()
	var match []cadres.Cadre
	for _, c := range list {
		if (isDirArg(arg) && c.Path == paths.Real(arg)) || strings.EqualFold(c.Name, arg) {
			match = append(match, c)
		}
	}
	switch {
	case len(match) == 0:
		return e.fail("%s is not a known cadre (see cadre ls --all)", arg)
	case len(match) > 1:
		return e.fail("several known cadres are named %s; give the path instead", arg)
	case !match[0].External:
		return e.fail("%s lives in ~/.cadre; cadre cadres remove only forgets cadres kept outside it", match[0].Name)
	case match[0].Name == cadres.Default() && match[0].Present():
		return e.fail("%s is the default cadre; make another one the default first (cadre use <name>)", match[0].Name)
	}
	if err := cadres.RemoveExternal(match[0].Path); err != nil {
		return e.fail("%s", err)
	}
	e.say("forgot cadre %s (%s); its folder is untouched", match[0].Name, match[0].Path)
	return 0
}

// partName is a team or role name: part of session names and of paths.
var partName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func runTeamAdd(e *env) int {
	if len(e.args) != 1 {
		return e.fail("usage: cadre team add <team>")
	}
	if !partName.MatchString(e.args[0]) {
		return e.fail("a team's name may use letters, digits, - and _")
	}
	if e.persona("add teams") {
		return 1
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	if err := os.MkdirAll(filepath.Join(r.Path, "personas", e.args[0]), 0o755); err != nil {
		return e.fail("%s", err)
	}
	e.say("  team %s added; add personas with cadre persona add %s/<role>", e.args[0], e.args[0])
	return 0
}

func runPersonaAdd(e *env) int {
	team, role, ok := strings.Cut(strings.Join(e.args, " "), "/")
	if len(e.args) != 1 || !ok {
		return e.fail("usage: cadre persona add <team>/<role>")
	}
	if !partName.MatchString(team) || !partName.MatchString(role) {
		return e.fail("a team's and a role's name may use letters, digits, - and _")
	}
	// A new persona's prompt is launched later with the cadre's grants.
	if e.persona("add personas") {
		return 1
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	f := filepath.Join(r.Path, "personas", team, role+".md")
	if _, err := os.Lstat(f); err == nil {
		return e.fail("%s already exists", f)
	}
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return e.fail("%s", err)
	}
	body := fmt.Sprintf("# Persona: %s (%s team)\n\nYou are the %s. Describe what you own, how you work, and what your reply to the orchestrator contains.\n", role, team, role)
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		return e.fail("%s", err)
	}
	note, err := cadres.Commit(r.Path, "Add persona "+team+"/"+role, f)
	if err != nil {
		return e.fail("%s", err)
	}
	if note != "" {
		e.say("%s", note)
	}
	e.say("  created %s; edit it to describe the role", f)
	return 0
}

// projectDir finds a registered project's folder.
func projectDir(r *cadres.Resolved, name string) (string, error) {
	reg, err := registry.Load(r.Registry())
	if err != nil {
		return "", err
	}
	entry := reg.Get(name)
	if entry == nil {
		return "", errNotRegistered
	}
	d := cadres.ProjectDir(r.Cadre, entry)
	if d == "" {
		return "", fmt.Errorf("project '%s' has no folder yet; set where clones go with cadre project dir <dir>", name)
	}
	return d, nil
}

var errNotRegistered = errors.New("not registered")

func runProjectPath(e *env) int {
	if len(e.args) != 1 {
		return e.fail("usage: cadre project path <name>")
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	d, err := projectDir(r, e.args[0])
	switch {
	case errors.Is(err, errNotRegistered):
		// As in 0.1.x, a folder is printed as it is.
		if st, serr := os.Stat(e.args[0]); serr == nil && st.IsDir() {
			abs, _ := filepath.Abs(e.args[0])
			e.say("%s", abs)
			return 0
		}
		return e.fail("'%s' is neither a registry project nor a folder", e.args[0])
	case err != nil:
		return e.fail("%s", err)
	}
	if st, err := os.Stat(d); err != nil || !st.IsDir() {
		return e.fail("project '%s' is not at %s (run cadre project sync)", e.args[0], d)
	}
	e.say("%s", d)
	return 0
}

// runProjects is the 0.1.x listing, kept as an old name: a ? marks a
// project missing on this machine.
func runProjects(e *env) int {
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	reg, err := registry.Load(r.Registry())
	if err != nil {
		return e.fail("%s", err)
	}
	for _, entry := range reg.Entries() {
		mark := "  "
		if d := cadres.ProjectDir(r.Cadre, entry); d == "" {
			mark = "? "
		} else if st, err := os.Stat(d); err != nil || !st.IsDir() {
			mark = "? "
		}
		e.say("%s%-14s %-8s %s", mark, entry.Name, entry.Get("team"), entry.Get("about"))
	}
	return 0
}
