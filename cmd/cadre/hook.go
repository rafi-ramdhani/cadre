//go:build !windows

package main

import (
	"os"
	"strings"

	cadre "github.com/rafi-ramdhani/cadre"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
)

// runHookOrchestrator is the hidden cadre hook orchestrator: the session
// start hook that makes a new session the orchestrator. It stays silent
// in a persona's session, with CADRE_OFF, and in an orchestrator cadre
// started (which has the text in its prompt already), and it never fails:
// a hook that errors would get in the way of every session.
func runHookOrchestrator(e *env) int {
	for _, v := range []string{"CADRE_PERSONA", "CADRE_OFF", "CADRE_ORCHESTRATOR"} {
		if os.Getenv(v) != "" {
			return 0
		}
	}
	rt, err := runtime.Get(runtimeName())
	if err != nil {
		return 0
	}
	text, _ := cadre.Assets.ReadFile("orchestrator.md")
	e.stdout.Write(rt.Hooks().Output(strings.TrimSpace(string(text))))
	return 0
}
