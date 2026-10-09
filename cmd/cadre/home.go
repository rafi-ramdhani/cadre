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
	if e.lines == nil {
		e.lines = bufio.NewReader(e.stdin)
	}
	line, _ := e.lines.ReadString('\n')
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
	if len(e.args) != 1 || strings.HasPrefix(e.args[0], "-") {
		return e.fail("usage: cadre init <name>")
	}
	if e.persona("create or switch cadres") || !e.home() {
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
	if e.persona("create or switch cadres") || !e.home() {
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
