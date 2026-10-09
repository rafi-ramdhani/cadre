//go:build !windows

package main

import (
	"errors"
	"fmt"
	"strings"
)

// A command as the user types it. The table below is the single source for
// dispatch, `cadre help` and `cadre help advanced` (section M.2).
type command struct {
	name    string // the words that select it: "ls", "project add", "--version"; "" is plain cadre
	usage   string // its arguments, for help
	summary string
	group   string // "" for a visible command, else its group in help advanced
	hidden  bool   // in neither help screen (the hook subcommands)
	run     func(e *env) int
}

// An old name from 0.1.x: still works, silently, as the command it maps to.
type oldName struct {
	name string // as typed
	now  string // the command it runs, for help advanced
}

// A name from unreleased 0.2.0 work that was renamed before release: it
// gets no alias, only a pointer to the new name.
type renamed struct {
	name string
	use  string
}

var groups = []string{"Cadres", "Projects", "Teams", "Permissions", "Context", "Framework", "Data"}

var commands []command

var oldNames = []oldName{
	{"init", "cadre init"},
	{"use", "cadre use"},
	{"up", "cadre up"},
	{"down", "cadre stop"},
	{"down --all", "cadre stop"},
	{"attach", "cadre attach"},
	{"ls", "cadre ls"},
	{"projects", "the projects line of cadre ls"},
	{"path", "cadre project path"},
	{"sync", "cadre project sync"},
	{"add project", "cadre project add"},
	{"add team", "cadre team add"},
	{"add persona", "cadre persona add"},
	{"version", "cadre --version"},
	{"-v", "cadre --version"},
	{"-h", "cadre help"},
	{"--help", "cadre help"},
}

var renames = []renamed{
	{"which", "cadre ls"},
	{"cadres", "cadre ls (cadre cadres add|remove manage outside cadres)"},
	{"cadres list", "cadre ls"},
	{"ctx", "cadre ls"},
	{"start", "cadre --tmux"},
	{"trust", "cadre project trust"},
	{"remove project", "cadre project unlink (or cadre project move)"},
	{"export project", "cadre project export"},
	{"project remove", "cadre project unlink (or cadre project move)"},
}

func init() {
	commands = []command{
		// Visible (section M.2).
		{name: "", summary: "open this cadre's orchestrator in this terminal", run: notBuilt("cadre")},
		{name: "ls", usage: "[--all]", summary: "what runs, your projects and your other cadres", run: runLs},
		{name: "attach", usage: "<team> [project]", summary: "watch or talk to a running team (tmux)", run: runAttach},
		{name: "stop", usage: "[team[/role]] [project] [--all]", summary: "stop a team; with no team, every team of this cadre (asks first)", run: runStop},
		{name: "help", usage: "[advanced]", summary: "these commands; advanced lists the rest", run: runHelp},
		{name: "--version", summary: "the version and how cadre was installed", run: runVersion},
		{name: "uninstall", usage: "[--yes | --dry-run]", summary: "remove cadre's links, skill, hook and config (keeps your cadres and projects)", run: notBuilt("cadre uninstall")},

		// Advanced, grouped.
		{name: "init", usage: "<name>", summary: "create a cadre in ~/.cadre/<name>", group: "Cadres", run: runInit},
		{name: "use", usage: "<name>", summary: "make a cadre the default", group: "Cadres", run: runUse},
		{name: "cadres add", usage: "<dir>", summary: "use a cadre kept outside ~/.cadre", group: "Cadres", run: runCadresAdd},
		{name: "cadres remove", usage: "<name|dir>", summary: "stop using an outside cadre (its folder is kept)", group: "Cadres", run: runCadresRemove},
		{name: "migrate", usage: "[--dry-run]", summary: "move a visible cadre into ~/.cadre (projects stay put)", group: "Cadres", run: notBuilt("cadre migrate")},

		{name: "project add", usage: "<name> <repo> | <name> --path <dir>", summary: "clone a project into your projects folder, or link a folder", group: "Projects", run: runProjectAdd},
		{name: "project unlink", usage: "<name> [--untrust]", summary: "remove a project from the registry (its folder is kept)", group: "Projects", run: notBuilt("cadre project unlink")},
		{name: "project move", usage: "<name> <dir>", summary: "move a project's folder and keep it linked", group: "Projects", run: notBuilt("cadre project move")},
		{name: "project relink", usage: "<name> <dir>", summary: "point a project at its folder's new place", group: "Projects", run: notBuilt("cadre project relink")},
		{name: "project restore", usage: "<name>", summary: "clone a missing project again, to the same place", group: "Projects", run: notBuilt("cadre project restore")},
		{name: "project export", usage: "<name> <dir>", summary: "a clean git copy of a project", group: "Projects", run: notBuilt("cadre project export")},
		{name: "project trust", usage: "<name> | --all", summary: "mark project folders as trusted, so personas start there without asking", group: "Projects", run: runProjectTrust},
		{name: "project sync", summary: "clone registry projects missing on this machine", group: "Projects", run: runProjectSync},
		{name: "project path", usage: "<name>", summary: "print a project's folder", group: "Projects", run: runProjectPath},
		{name: "project dir", usage: "[<dir>]", summary: "where new clones go", group: "Projects", run: runProjectDir},

		{name: "up", usage: "<team|team/role> [project|dir]", summary: "start a team or one persona", group: "Teams", run: runUp},
		{name: "team add", usage: "<team>", summary: "add a team", group: "Teams", run: runTeamAdd},
		{name: "persona add", usage: "<team>/<role>", summary: "add a persona", group: "Teams", run: runPersonaAdd},

		{name: "allow", usage: "[list]", summary: "the grants every persona gets", group: "Permissions", run: runAllowList},
		{name: "allow add", usage: "[--once] <rule> | [--once] --auto \"<text>\"", summary: "grant a rule or a plain-English allowance to personas", group: "Permissions", run: runAllowAdd},
		{name: "allow remove", usage: "<rule|number|--once>", summary: "remove a grant, or every one-time grant", group: "Permissions", run: runAllowRemove},

		{name: "compact", usage: "<team/role> [project]", summary: "compact a persona's conversation", group: "Context", run: notBuilt("cadre compact")},

		{name: "update", usage: "[--check]", summary: "update cadre", group: "Framework", run: notBuilt("cadre update")},
		{name: "--check", summary: "the full health check", group: "Framework", run: notBuilt("cadre --check")},
		{name: "--tmux", usage: "[--detach]", summary: "open the orchestrator in tmux", group: "Framework", run: notBuilt("cadre --tmux")},

		{name: "ls --json", summary: "the status screen's data, for the orchestrator", group: "Data", run: runLsJSON},

		// Hidden: the 0.1.x projects listing, kept as an old name (M.2).
		{name: "projects", hidden: true, run: runProjects},

		// Hidden: run by Claude Code as hooks and the status line (O.5).
		{name: "hook orchestrator", hidden: true, run: notBuilt("cadre hook orchestrator")},
		{name: "hook statusline", hidden: true, run: notBuilt("cadre hook statusline")},
		{name: "hook state", hidden: true, run: notBuilt("cadre hook state")},
		{name: "hook session", hidden: true, run: notBuilt("cadre hook session")},
	}
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
// the arguments after it. Old names run their new command; renamed
// unreleased names are refused with the new name.
func lookup(args []string) (command, []string, error) {
	// Unreleased forms of old names get no alias.
	if len(args) > 0 && args[0] == "down" && contains(args, "--all-cadres") {
		return command{}, nil, errors.New("down --all --all-cadres is now cadre stop --all")
	}
	if len(args) > 0 && args[0] == "ls" && contains(args, "--all-cadres") {
		return command{}, nil, errors.New("ls --all-cadres is now cadre ls --all")
	}
	if matches("add project", args) > 0 && contains(args, "--path") {
		return command{}, nil, errors.New("add project --path is now cadre project add <name> --path <dir>")
	}
	if alias, rest, ok := matchOld(args); ok {
		args = append(strings.Fields(alias), rest...)
	}
	for _, r := range renames {
		if n := matches(r.name, args); n > 0 && (r.name != "cadres" || len(args) == 1) {
			return command{}, nil, fmt.Errorf("%s is not a command; use %s", r.name, r.use)
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
// "project" or "cadres".
func isGroupWord(word string) bool {
	for _, c := range commands {
		if w := strings.Fields(c.name); len(w) > 1 && w[0] == word {
			return true
		}
	}
	return false
}

// matchOld maps an old name to the words of the command it now runs.
func matchOld(args []string) (string, []string, bool) {
	words := map[string]string{
		"down": "stop", "path": "project path", "sync": "project sync",
		"add project": "project add", "add team": "team add", "add persona": "persona add",
		"version": "--version", "-v": "--version", "-h": "help", "--help": "help",
	}
	for _, two := range []string{"add project", "add team", "add persona"} {
		if n := matches(two, args); n > 0 {
			return words[two], args[n:], true
		}
	}
	if len(args) > 0 {
		if now, ok := words[args[0]]; ok {
			rest := args[1:]
			if args[0] == "down" {
				rest = without(rest, "--all") // down --all is stop with no team
			}
			return now, rest, true
		}
	}
	return "", nil, false
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
		e.say("\nOld names (still work):")
		for _, o := range oldNames {
			e.say("  cadre %-30s %s", o.name, o.now)
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
