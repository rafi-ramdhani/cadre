//go:build !windows

package main

import (
	"fmt"
	"strings"
)

// A command as the user types it. The table below is the single source for
// dispatch, `cadre help` and `cadre help advanced`.
type command struct {
	name    string // the words that select it: "ls", "project add", "--version"; "" is plain cadre
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

var groups = []string{"Cadres", "Projects", "Sessions", "Permissions", "Health", "Data"}

var commands []command

var oldNames = []oldName{
	{"down", "use cadre stop"},
	{"projects", "use cadre ls"},
	{"path", "use cadre project path"},
	{"sync", "use cadre project sync"},
	{"add project", "ask the orchestrator, or use cadre project add"},
	{"add team", "ask the orchestrator to add the team"},
	{"add persona", "ask the orchestrator to add the persona"},
	{"version", "use cadre --version"},
}

func init() {
	commands = []command{
		// Visible.
		{name: "", summary: "open this cadre's orchestrator in this terminal", run: runPlain},
		{name: "--tmux", usage: "[--detach]", summary: "open the orchestrator in tmux, to come back to later", run: runTmux},
		{name: "ls", usage: "[--all]", summary: "what runs, your projects and your other cadres", run: runLs},
		{name: "attach", usage: "[team] [project]", summary: "watch or talk to a running team, or the orchestrator in tmux", run: runAttach},
		{name: "stop", usage: "[team[/role]] [project] [--all]", summary: "stop a team; with no team, every team of this cadre (asks first)", run: runStop},
		{name: "help", usage: "[advanced]", summary: "these commands; advanced lists the rest", run: runHelp},
		{name: "--version", summary: "the version", run: runVersion},
		{name: "uninstall", usage: "[--yes | --dry-run]", summary: "remove cadre's skill link, hook and config (keeps your cadres and projects)", run: notBuilt("cadre uninstall")},

		// Advanced, grouped.
		{name: "init", usage: "<name>", summary: "create a cadre in ~/.cadre/<name>", group: "Cadres", run: runInit},
		{name: "use", usage: "<name>", summary: "make a cadre the default", group: "Cadres", run: runUse},
		{name: "migrate", usage: "[--dry-run]", summary: "move a 0.1.x cadre into ~/.cadre (projects stay put)", group: "Cadres", run: notBuilt("cadre migrate")},

		{name: "project add", usage: "<name> <repo> | <name> --path <dir>", summary: "clone a project into your projects folder, or link a folder", group: "Projects", run: runProjectAdd},
		{name: "project link", usage: "<name> <dir>", summary: "set where a project's folder is on this machine", group: "Projects", run: runProjectLink},
		{name: "project unlink", usage: "<name> [--untrust]", summary: "remove a project from the registry (its folder is kept)", group: "Projects", run: runProjectUnlink},
		{name: "project sync", summary: "clone registry projects missing on this machine", group: "Projects", run: runProjectSync},
		{name: "project trust", usage: "<name> | --all", summary: "mark project folders as trusted, so personas start there without asking", group: "Projects", run: runProjectTrust},
		{name: "project path", usage: "<name>", summary: "print a project's folder", group: "Projects", run: runProjectPath},
		{name: "project dir", usage: "[<dir>]", summary: "where new clones go", group: "Projects", run: runProjectDir},

		{name: "up", usage: "<team|team/role> [project|dir]", summary: "start a team or one persona", group: "Sessions", run: runUp},

		{name: "allow", usage: "[list]", summary: "the grants every persona gets", group: "Permissions", run: runAllowList},
		{name: "allow add", usage: "[--once] <rule> | [--once] --auto \"<text>\"", summary: "grant a rule or a plain-English allowance to personas", group: "Permissions", run: runAllowAdd},
		{name: "allow remove", usage: "<rule|number|--once>", summary: "remove a grant, or every one-time grant", group: "Permissions", run: runAllowRemove},

		{name: "--check", summary: "the full health check", group: "Health", run: runCheck},

		{name: "ls --json", summary: "the status screen's data, for the orchestrator", group: "Data", run: runLsJSON},

		// Hidden: run by Claude Code and git as hooks.
		{name: "hook orchestrator", hidden: true, run: runHookOrchestrator},
		{name: "hook session", hidden: true, run: notBuilt("cadre hook session")},
		{name: "hook pre-push", hidden: true, run: notBuilt("cadre hook pre-push")},
	}
}

// runTmux is plain cadre with the flag its entry takes.
func runTmux(e *env) int {
	e.args = append([]string{"--tmux"}, e.args...)
	return runPlain(e)
}

// runLsJSON is cadre ls --json: the table's entry takes the flag.
func runLsJSON(e *env) int {
	e.args = append([]string{"--json"}, e.args...)
	return runLs(e)
}

// notBuilt stands in for a command until its porting step lands.
func notBuilt(name string) func(e *env) int {
	return func(e *env) int { return e.fail("%s is not built yet in this version", name) }
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
			return command{}, nil, fmt.Errorf("%s is not a command since cadre 0.2.0; %s", o.name, o.use)
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
			return command{}, nil, fmt.Errorf("unknown command '%s %s' (see cadre help advanced)", args[0], args[1])
		}
		return command{}, nil, fmt.Errorf("unknown command '%s' (see cadre help)", args[0])
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
// it does not match. The empty name (plain cadre) matches only no args.
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
		return e.fail("usage: cadre --version")
	}
	e.say("cadre %s", version)
	return 0
}

func runHelp(e *env) int {
	switch {
	case len(e.args) == 0:
		e.say("cadre: a team of Claude Code sessions you lead from one conversation\n")
		for _, c := range commands {
			if c.group == "" && !c.hidden {
				line(e, c)
			}
		}
		e.say("\nAsk the orchestrator for everything else, in plain words.")
		e.say("Commands act on the cadre of the project you are in, else your default cadre.")
		e.say("All commands: cadre help advanced")
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
		return 0
	}
	return e.fail("usage: cadre help [advanced]")
}

func line(e *env, c command) {
	words := strings.TrimSpace("cadre " + c.name + " " + c.usage)
	if len(words) > 44 {
		e.say("  %s\n  %-44s %s", words, "", c.summary)
		return
	}
	e.say("  %-44s %s", words, c.summary)
}
