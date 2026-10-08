package jsonx

import (
	"path/filepath"

	"github.com/rafi-ramdhani/cadre/internal/shellwords"
)

// OrchestratorHook returns the program a cadre orchestrator hook command
// runs: the script of a 0.1.x `bash <framework>/bin/orchestrator-hook.sh`
// entry, or the binary of a `<cadre binary> hook orchestrator` entry. The
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
	case len(words) == 3 && filepath.Base(words[0]) == "cadre" && words[1] == "hook" && words[2] == "orchestrator":
		return words[0], true
	}
	return "", false
}
