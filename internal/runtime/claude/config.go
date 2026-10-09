package claude

import (
	"path/filepath"

	"github.com/rafi-ramdhani/cadrei/internal/jsonx"
	"github.com/rafi-ramdhani/cadrei/internal/shellwords"
)

// Edits of Claude Code's own JSON files, ~/.claude.json and
// ~/.claude/settings.json, through jsonx.Edit's safe write.

// TrustEntry names a project and the folder keys to trust it under (its
// physical path and, when different, the path as cadrei spells it).
type TrustEntry struct {
	Name string
	Dirs []string
}

// Trust marks folders as trusted in Claude Code's config
// (projects[<dir>].hasTrustDialogAccepted = true). It reports
// "<name>\ttrusted" or "<name>\talready" for each entry.
func Trust(entries []TrustEntry) jsonx.Op {
	return func(root *jsonx.Value) ([]string, bool, error) {
		projects, err := projectsOf(root, true)
		if err != nil {
			return nil, false, err
		}
		var lines []string
		changed := false
		for _, e := range entries {
			all := true
			for _, d := range e.Dirs {
				if !projects.Get(d).Get("hasTrustDialogAccepted").IsTrue() {
					all = false
				}
			}
			if all {
				lines = append(lines, e.Name+"\talready")
				continue
			}
			for _, d := range e.Dirs {
				entry := projects.Get(d)
				if entry == nil {
					entry = jsonx.NewObject()
					projects.Set(d, entry)
				}
				if entry.Kind != jsonx.Object {
					return nil, false, &jsonx.ShapeError{What: "projects entry is not an object"}
				}
				entry.Set("hasTrustDialogAccepted", jsonx.Literal("true"))
			}
			lines = append(lines, e.Name+"\ttrusted")
			changed = true
		}
		return lines, changed, nil
	}
}

// Untrust removes hasTrustDialogAccepted from the folders' entries and
// leaves every other key (section L). It reports "<name>\tuntrusted" or
// "<name>\tnot trusted".
func Untrust(entries []TrustEntry) jsonx.Op {
	return func(root *jsonx.Value) ([]string, bool, error) {
		projects, err := projectsOf(root, false)
		if err != nil {
			return nil, false, err
		}
		var lines []string
		changed := false
		for _, e := range entries {
			removed := false
			for _, d := range e.Dirs {
				entry := projects.Get(d)
				if entry == nil {
					continue
				}
				if entry.Kind != jsonx.Object {
					return nil, false, &jsonx.ShapeError{What: "projects entry is not an object"}
				}
				if entry.Delete("hasTrustDialogAccepted") {
					removed = true
				}
			}
			if removed {
				lines = append(lines, e.Name+"\tuntrusted")
				changed = true
			} else {
				lines = append(lines, e.Name+"\tnot trusted")
			}
		}
		return lines, changed, nil
	}
}

// projectsOf returns root's projects object, made when create is set.
func projectsOf(root *jsonx.Value, create bool) (*jsonx.Value, error) {
	if root.Kind != jsonx.Object {
		return nil, &jsonx.ShapeError{What: "the top level is not an object"}
	}
	projects := root.Get("projects")
	if projects == nil {
		projects = jsonx.NewObject()
		if create {
			root.Set("projects", projects)
		}
	}
	if projects.Kind != jsonx.Object {
		return nil, &jsonx.ShapeError{What: "projects is not an object"}
	}
	return projects, nil
}

// sessionStart returns the SessionStart groups of a settings file, made
// when create is set, after checking the shape on the way.
func sessionStart(root *jsonx.Value, create bool) (hooks, groups *jsonx.Value, err error) {
	if root.Kind != jsonx.Object {
		return nil, nil, &jsonx.ShapeError{What: "the top level is not an object"}
	}
	hooks = root.Get("hooks")
	if hooks == nil {
		if !create {
			return nil, nil, nil
		}
		hooks = jsonx.NewObject()
		root.Set("hooks", hooks)
	}
	if hooks.Kind != jsonx.Object {
		return nil, nil, &jsonx.ShapeError{What: "hooks is not an object"}
	}
	groups = hooks.Get("SessionStart")
	if groups == nil {
		if !create {
			return hooks, nil, nil
		}
		groups = &jsonx.Value{Kind: jsonx.Array, Items: []*jsonx.Value{}}
		hooks.Set("SessionStart", groups)
	}
	if groups.Kind != jsonx.Array {
		return nil, nil, &jsonx.ShapeError{What: "hooks.SessionStart is not a list"}
	}
	for _, g := range groups.Items {
		if g.Kind != jsonx.Object || g.Get("hooks") == nil || g.Get("hooks").Kind != jsonx.Array {
			return nil, nil, &jsonx.ShapeError{What: "a SessionStart group has no hooks list"}
		}
	}
	return hooks, groups, nil
}

// commandOf returns a hook's command, if it has one.
func commandOf(h *jsonx.Value) (string, bool) {
	if h.Kind != jsonx.Object {
		return "", false
	}
	return h.Get("command").Text()
}

// AddHook makes command the SessionStart hook for which isCadrei is true:
// groups holding any such hook are dropped and one group with command is
// added. Unchanged when command is already the only one.
func AddHook(command string, isCadrei func(command string) bool) jsonx.Op {
	return func(root *jsonx.Value) ([]string, bool, error) {
		_, groups, err := sessionStart(root, true)
		if err != nil {
			return nil, false, err
		}
		var found []string
		for _, g := range groups.Items {
			for _, h := range g.Get("hooks").Items {
				if c, ok := commandOf(h); ok && isCadrei(c) {
					found = append(found, c)
				}
			}
		}
		if len(found) == 1 && found[0] == command {
			return []string{"already"}, false, nil
		}
		kept := []*jsonx.Value{}
		for _, g := range groups.Items {
			ours := false
			for _, h := range g.Get("hooks").Items {
				if c, ok := commandOf(h); ok && isCadrei(c) {
					ours = true
				}
			}
			if !ours {
				kept = append(kept, g)
			}
		}
		hook := jsonx.NewObject("type", jsonx.String("command"), "command", jsonx.String(command), "timeout", jsonx.Literal("10"))
		group := jsonx.NewObject("hooks", &jsonx.Value{Kind: jsonx.Array, Items: []*jsonx.Value{hook}})
		groups.Items = append(kept, group)
		return []string{"added"}, true, nil
	}
}

// Unhook removes the SessionStart hooks for which isOurs is true, and
// groups left empty by that. It reports "removed\t<command>" for each, and
// "kept\t<command>" for hooks isCadrei recognises as another cadrei
// framework's.
func Unhook(isOurs, isCadrei func(command string) bool) jsonx.Op {
	return func(root *jsonx.Value) ([]string, bool, error) {
		hooks, groups, err := sessionStart(root, false)
		if err != nil || groups == nil {
			return nil, false, err
		}
		var removed, others []string
		kept := []*jsonx.Value{}
		for _, g := range groups.Items {
			list := g.Get("hooks")
			rest := []*jsonx.Value{}
			for _, h := range list.Items {
				c, ok := commandOf(h)
				switch {
				case ok && isOurs(c):
					removed = append(removed, c)
				case ok && isCadrei(c):
					others = append(others, c)
					rest = append(rest, h)
				default:
					rest = append(rest, h)
				}
			}
			if len(rest) == len(list.Items) {
				kept = append(kept, g)
			} else if len(rest) > 0 {
				list.Items = rest
				kept = append(kept, g)
			}
		}
		var lines []string
		for _, c := range removed {
			lines = append(lines, "removed\t"+c)
		}
		for _, c := range others {
			lines = append(lines, "kept\t"+c)
		}
		if len(removed) == 0 {
			return lines, false, nil
		}
		if len(kept) > 0 {
			groups.Items = kept
		} else {
			hooks.Delete("SessionStart")
			if len(hooks.Members) == 0 {
				root.Delete("hooks")
			}
		}
		return lines, true, nil
	}
}

// OrchestratorHook returns the program a cadrei orchestrator hook command
// runs: the script of a 0.1.x `bash <framework>/bin/orchestrator-hook.sh`
// entry, or the binary of a `<cadrei binary> hook orchestrator` entry. The
// command is split as the shell splits it, so a quoted path with a space
// is read whole.
func OrchestratorHook(command string) (string, bool) {
	words, err := shellwords.Split(command)
	if err != nil {
		return "", false
	}
	switch {
	case len(words) == 2 && words[0] == "bash" && filepath.Base(words[1]) == "orchestrator-hook.sh":
		return words[1], true
	case len(words) == 3 && filepath.Base(words[0]) == "cadrei" && words[1] == "hook" && words[2] == "orchestrator":
		return words[0], true
	}
	return "", false
}
