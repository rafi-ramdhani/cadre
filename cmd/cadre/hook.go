//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	cadre "github.com/rafi-ramdhani/cadre"
	"github.com/rafi-ramdhani/cadre/internal/backup"
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

// runHookPrePush is the hidden cadre hook pre-push, which git runs before a
// cadre repository is pushed (its backup): it refuses a push that carries
// a file that looks like a credential or is over 50 MB, naming each one.
// When it cannot check, it refuses too: a backup can wait, a leaked token
// cannot be taken back.
func runHookPrePush(e *env) int {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return e.fail("the pre-push check runs in a git repository")
	}
	repo := strings.TrimSpace(string(out))
	remote := ""
	if len(e.args) > 0 {
		remote = e.args[0]
	}
	findings, err := backup.Scan(repo, remote, backup.ReadUpdates(e.stdin))
	if err != nil {
		return e.fail("could not check this push for credentials (%s), so it was stopped", err)
	}
	if len(findings) == 0 {
		return 0
	}
	fmt.Fprintln(e.stderr, "cadre: this push was stopped; it carries files that must not leave this machine:")
	for _, f := range findings {
		fmt.Fprintf(e.stderr, "  %s: %s\n", f.Path, f.Why)
	}
	fmt.Fprintln(e.stderr, "Take them out of the commits being pushed (the history, not only the last commit), then push again.")
	return 1
}
