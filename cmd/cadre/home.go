//go:build !windows

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	cadre "github.com/rafi-ramdhani/cadre"
	"github.com/rafi-ramdhani/cadre/internal/backup"
	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/framework"
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
	answer, _ := e.answer(question)
	return answer
}

// answer asks like ask, and reports false when the input has ended (Ctrl-D
// on a terminal): never an answer, so it never accepts a default.
func (e *env) answer(question string) (string, bool) {
	fmt.Fprint(e.stderr, question)
	if e.lines == nil {
		e.lines = bufio.NewReader(e.stdin)
	}
	line, err := e.lines.ReadString('\n')
	if err != nil && line == "" {
		e.eof = true
		fmt.Fprintln(e.stderr)
		return "", false
	}
	return strings.TrimSpace(line), true
}

// resolve finds the cadre this command acts on (N.3), asking which one
// when several cadres link the project the user is in.
func (e *env) resolve() (*cadres.Resolved, bool) {
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

// inMember reports whether cadre runs in a member's session: CADRE_MEMBER,
// which cadre sets, or CADRE_PERSONA, which sessions started by 0.1.x
// carry. CADRE_PERSONA is read only, never set, and is a temporary alias:
// it goes once no 0.1.x sessions are around.
func inMember() bool { return os.Getenv("CADRE_MEMBER") != "" || os.Getenv("CADRE_PERSONA") != "" }

// member refuses a command in a member's session.
func (e *env) member(why string) bool {
	if inMember() {
		e.fail("refused for members: members cannot %s; ask the user in the orchestrator", why)
		return true
	}
	return false
}

func runInit(e *env) int {
	if len(e.args) != 1 || strings.HasPrefix(e.args[0], "-") {
		return e.fail("usage: cadre init <name>")
	}
	if e.member("create or switch cadres") {
		return 1
	}
	c, note, err := cadres.Create(e.args[0], cadre.Assets)
	if err != nil {
		return e.fail("%s", err)
	}
	e.say("created %s", c.Path)
	if note != "" {
		e.say("%s", note)
	}
	e.guard(c)
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

func runUse(e *env) int {
	if len(e.args) != 1 || strings.HasPrefix(e.args[0], "-") {
		return e.fail("usage: cadre use <name>")
	}
	if e.member("create or switch cadres") {
		return 1
	}
	c, ok := cadres.Find(e.args[0])
	if !ok || !c.Present() {
		return e.fail("no cadre named %s in ~/.cadre (see cadre ls --all)", e.args[0])
	}
	if err := cadres.SetDefault(c.Name); err != nil {
		return e.fail("%s", err)
	}
	e.say("default cadre: %s (%s)", c.Name, c.Path)
	return 0
}

// projectDir finds a registered project's folder on this machine, or
// says why it has none: not here yet, gone, or on a drive that is not
// connected. Cadre never unlinks a missing project by itself.
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
	switch cadres.Where(d) {
	case "not here":
		return "", fmt.Errorf("project '%s' is not on this machine yet; clone it with cadre project sync, or link its folder with cadre project link %s <dir>", name, name)
	case "drive":
		return "", fmt.Errorf("project '%s' is on a drive that is not connected (%s); connect the drive", name, display(d))
	case "missing":
		return "", fmt.Errorf("project '%s' is missing: %s is gone; clone it again with cadre project sync, link its new folder with cadre project link %s <dir>, or unlink it", name, display(d), name)
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
	e.say("%s", d)
	return 0
}

// guard installs cadre's pre-push guard in a cadre repository.
func (e *env) guard(c cadres.Cadre) {
	if _, err := backup.Install(c.Path, framework.Binary()); err != nil {
		fmt.Fprintf(e.stderr, "warning: cadre's check for credentials before a push is not installed: %s\n", err)
	}
}
