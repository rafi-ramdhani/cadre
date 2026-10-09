//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	cadrei "github.com/rafi-ramdhani/cadrei"
	"github.com/rafi-ramdhani/cadrei/internal/backup"
	"github.com/rafi-ramdhani/cadrei/internal/cadreis"
	"github.com/rafi-ramdhani/cadrei/internal/orchestrator"
	"github.com/rafi-ramdhani/cadrei/internal/paths"
	"github.com/rafi-ramdhani/cadrei/internal/runtime"
	"github.com/rafi-ramdhani/cadrei/internal/session"
)

// runHookOrchestrator is the hidden cadrei hook orchestrator: the session
// start hook that makes a new session the orchestrator. It stays silent
// in a member's session, with CADREI_OFF, and in an orchestrator cadrei
// started (which has the text in its prompt already), and it never fails:
// a hook that errors would get in the way of every session.
func runHookOrchestrator(e *env) int {
	if inMember() || os.Getenv("CADREI_OFF") != "" || os.Getenv("CADREI_ORCHESTRATOR") != "" {
		return 0
	}
	rt, err := runtime.Get(runtimeName())
	if err != nil {
		return 0
	}
	text, _ := cadrei.Assets.ReadFile("orchestrator.md")
	e.stdout.Write(rt.Hooks().Output(strings.TrimSpace(string(text))))
	return 0
}

// runHookPrePush is the hidden cadrei hook pre-push, which git runs before a
// cadrei repository is pushed (its backup): it refuses a push that carries
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
	fmt.Fprintln(e.stderr, "cadrei: this push was stopped; it carries files that must not leave this machine:")
	for _, f := range findings {
		fmt.Fprintf(e.stderr, "  %s: %s\n", f.Path, f.Why)
	}
	fmt.Fprintln(e.stderr, "Take them out of the commits being pushed (the history, not only the last commit), then push again.")
	return 1
}

// runHookSession is the hidden cadrei hook session, the session start hook
// in every member's settings copy: when the member's session starts, or
// starts over after /clear or /compact, it records the conversation id it
// now has, so the next start resumes the right conversation. It writes
// only a record of a cadrei that cadrei knows, for a name cadrei makes, and it
// never fails.
func runHookSession(e *env) int {
	name, home := os.Getenv("CADREI_MEMBER"), os.Getenv("CADREI_HOME")
	if name == "" || home == "" {
		return 0
	}
	var own *cadreis.Cadrei
	list, _ := cadreis.List()
	for i, c := range list {
		if c.Path == paths.Real(home) {
			own = &list[i]
		}
	}
	rt, err := runtime.Get(runtimeName())
	// Only a member's record: the orchestrator's is cadrei's own, written
	// at launch, and a name that is no team and role of this cadrei is no
	// member's.
	if own == nil || err != nil || name == orchestrator.Name(own.Name) || !memberOf(*own, name) {
		return 0
	}
	record := session.RecordPath(rt.BuildDir(paths.Real(home)), name)
	id := rt.Sessions().FromHook(e.stdin)
	if record == "" || id == "" {
		return 0
	}
	dir, _ := paths.Getwd()
	if r := session.ReadRecord(record); r != nil {
		if r.ID == id {
			return 0
		}
		dir = r.Dir // the folder it was started in
	}
	session.WriteRecord(record, session.Record{ID: id, Dir: dir, Since: time.Now()})
	return 0
}

// memberOf reports whether name is a member session name of cadrei c:
// <cadrei>-<team>-<role> or <cadrei>-<team>-<project>-<role>, for a team
// folder and a role file that exist.
func memberOf(c cadreis.Cadrei, name string) bool {
	teams, _ := os.ReadDir(filepath.Join(c.Path, "members"))
	for _, t := range teams {
		if !t.IsDir() || strings.HasPrefix(t.Name(), ".") {
			continue
		}
		prefix := c.Name + "-" + t.Name() + "-"
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		for _, role := range session.Roles(c.Path, t.Name()) {
			rest := strings.TrimPrefix(name, prefix)
			if rest == role || strings.HasSuffix(rest, "-"+role) && len(rest) > len(role)+1 {
				return true
			}
		}
	}
	return false
}
