//go:build !windows

package main

import (
	"fmt"
	"strings"

	"github.com/rafi-ramdhani/cadrei/internal/framework"
)

// A command as the user types it. The table below is the single source for
// dispatch, `cadrei help` and `cadrei help advanced`.
type command struct {
	name    string // the words that select it: "ls", "project add", "--version"; "" is plain cadrei
	usage   string // its arguments, for help
	summary string
	group   string // "" for a visible command, else its group in help advanced
	hidden  bool   // in neither help screen (the hook subcommands)
	run     func(e *env) int
}

// An old 0.1.x name: not an alias, only a one-line pointer to the new way.
type oldName struct {
	name string // as typed
	use  string // what to do now
}

var groups = []string{"Cadreis", "Projects", "Sessions", "Permissions", "Health", "Data"}

var commands []command

var oldNames = []oldName{
	{"down", "use cadrei stop"},
	{"projects", "use cadrei ls"},
	{"path", "use cadrei project path"},
	{"sync", "use cadrei project sync"},
	{"add project", "ask the orchestrator, or use cadrei project add"},
	{"add team", "ask the orchestrator to add the team"},
	{"add persona", "ask the orchestrator to add the member"},
	{"version", "use cadrei --version"},
	{"trust", "use cadrei project trust"},
	{"update", "update cadrei with brew upgrade cadrei, or by running install.sh again"},
}

func init() {
	commands = []command{
		// Visible.
		{name: "", usage: "[--fresh]", summary: "open this cadrei's orchestrator in this terminal", run: runPlain},
		{name: "--tmux", usage: "[--detach]", summary: "open the orchestrator in tmux, to come back to later", run: runTmux},
		{name: "ls", usage: "[--all]", summary: "what is running, your projects and your other cadreis", run: runLs},
		{name: "attach", usage: "[team] [project]", summary: "watch or talk to a running team, or the orchestrator in tmux", run: runAttach},
		{name: "stop", usage: "[team[/role]] [project] [--all] [--yes] [--fresh]", summary: "stop a team; with no team, every member of this cadrei (asks first)", run: runStop},
		{name: "help", usage: "[advanced]", summary: "these commands; advanced lists the rest", run: runHelp},
		{name: "--version", summary: "the version, and how cadrei was installed", run: runVersion},
		{name: "uninstall", usage: "[--yes | --dry-run]", summary: "remove cadrei's skill link, hook and config (keeps your cadreis and projects)", run: runUninstall},

		// Advanced, grouped.
		{name: "init", usage: "<name>", summary: "create a cadrei in ~/.cadrei/<name>", group: "Cadreis", run: runInit},
		{name: "use", usage: "<name>", summary: "make a cadrei the default", group: "Cadreis", run: runUse},

		{name: "project add", usage: "<name> <repo> | <name> --path <dir>", summary: "clone a project into your projects folder, or link a folder", group: "Projects", run: runProjectAdd},
		{name: "project link", usage: "<name> <dir>", summary: "set where a project's folder is on this machine", group: "Projects", run: runProjectLink},
		{name: "project unlink", usage: "<name> [--untrust]", summary: "remove a project from this cadrei (its folder is kept)", group: "Projects", run: runProjectUnlink},
		{name: "project sync", summary: "clone this cadrei's projects that are not on this machine yet", group: "Projects", run: runProjectSync},
		{name: "project trust", usage: "<name> | --all", summary: "mark project folders as trusted, so members start there without asking", group: "Projects", run: runProjectTrust},
		{name: "project path", usage: "<name>", summary: "print a project's folder", group: "Projects", run: runProjectPath},
		{name: "project dir", usage: "[<folder>]", summary: "where new clones go", group: "Projects", run: runProjectDir},

		{name: "up", usage: "<team|team/role> [project|dir] [--fresh]", summary: "start a team or one member", group: "Sessions", run: runUp},

		{name: "allow", usage: "[list]", summary: "the grants every member gets", group: "Permissions", run: runAllowList},
		{name: "allow add", usage: "[--once] <rule> | [--once] --auto \"<text>\"", summary: "grant a rule or a plain-English allowance to members", group: "Permissions", run: runAllowAdd},
		{name: "allow remove", usage: "<rule|number|--once>", summary: "remove a grant, or every one-time grant", group: "Permissions", run: runAllowRemove},

		{name: "--check", summary: "the full health check", group: "Health", run: runCheck},

		{name: "ls --json", summary: "the status screen's data, for the orchestrator", group: "Data", run: runLsJSON},

		// Hidden: run by Claude Code and git as hooks.
		{name: "hook orchestrator", hidden: true, run: runHookOrchestrator},
		{name: "hook session", hidden: true, run: runHookSession},
		{name: "hook pre-push", hidden: true, run: runHookPrePush},
	}
}

// runTmux is plain cadrei with the flag its entry takes.
func runTmux(e *env) int {
	e.args = append([]string{"--tmux"}, e.args...)
	return runPlain(e)
}

// runLsJSON is cadrei ls --json: the table's entry takes the flag.
func runLsJSON(e *env) int {
	e.args = append([]string{"--json"}, e.args...)
	return runLs(e)
}

// lookup finds the command for args, the longest name first, and returns
// the arguments after it. An old 0.1.x name is refused with the new way;
// -h, --help and -v are the usual flags for help and the version.
func lookup(args []string) (command, []string, error) {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help":
			args = append([]string{"help"}, args[1:]...)
		case "-v":
			args = append([]string{"--version"}, args[1:]...)
		}
	}
	for _, o := range oldNames {
		if matches(o.name, args) > 0 {
			return command{}, nil, fmt.Errorf("%s is not a command since cadrei 0.2.0; %s", o.name, o.use)
		}
	}
	best, bestLen := -1, -1
	for i, c := range commands {
		if n := matches(c.name, args); n > bestLen {
			best, bestLen = i, n
		}
	}
	if best < 0 || (bestLen == 0 && len(args) > 0) {
		if len(args) > 1 && isGroupWord(args[0]) {
			return command{}, nil, fmt.Errorf("unknown command '%s %s' (see cadrei help advanced)", args[0], args[1])
		}
		return command{}, nil, fmt.Errorf("unknown command '%s' (see cadrei help)", args[0])
	}
	return commands[best], args[bestLen:], nil
}

// isGroupWord reports whether word starts multi-word commands only, such as
// "project" or "allow".
func isGroupWord(word string) bool {
	for _, c := range commands {
		if w := strings.Fields(c.name); len(w) > 1 && w[0] == word {
			return true
		}
	}
	return false
}

// matches returns how many words of args a command name takes, or -1 when
// it does not match. The empty name (plain cadrei) matches only no args.
func matches(name string, args []string) int {
	if name == "" {
		if len(args) == 0 {
			return 0
		}
		return -1
	}
	words := strings.Fields(name)
	if len(args) < len(words) {
		return -1
	}
	for i, w := range words {
		if args[i] != w {
			return -1
		}
	}
	return len(words)
}

func runVersion(e *env) int {
	if len(e.args) > 0 {
		return e.fail("usage: cadrei --version")
	}
	e.say("cadrei %s (%s)", version, framework.Kind(version))
	return 0
}

func runHelp(e *env) int {
	switch {
	case len(e.args) == 0:
		e.say("cadrei: a team of Claude Code sessions you lead from one conversation\n")
		for _, c := range commands {
			if c.group == "" && !c.hidden {
				line(e, c)
			}
		}
		e.say("\nAsk the orchestrator for everything else, in plain words.")
		e.say("All commands: cadrei help advanced")
		return 0
	case len(e.args) == 1 && e.args[0] == "advanced":
		e.say("Advanced commands (the orchestrator runs these for you when you ask):")
		for _, g := range groups {
			e.say("\n%s", g)
			for _, c := range commands {
				if c.group == g {
					line(e, c)
				}
			}
		}
		e.say("\nCommands act on the cadrei of the folder or project you are in, else your default cadrei.")
		return 0
	}
	return e.fail("usage: cadrei help [advanced]")
}

func line(e *env, c command) {
	words := strings.TrimSpace("cadrei " + c.name + " " + c.usage)
	if len(words) > 44 {
		e.say("  %s\n  %-44s %s", words, "", c.summary)
		return
	}
	e.say("  %-44s %s", words, c.summary)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func without(list []string, s string) []string {
	out := make([]string, 0, len(list))
	for _, x := range list {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}
