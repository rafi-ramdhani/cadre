//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	cadre "github.com/rafi-ramdhani/cadre"
	"github.com/rafi-ramdhani/cadre/internal/backup"
	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/orchestrator"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
	"github.com/rafi-ramdhani/cadre/internal/session"
)

// runHookOrchestrator is the hidden cadre hook orchestrator: the session
// start hook that makes a new session the orchestrator. It stays silent
// in a member's session, with CADRE_OFF, and in an orchestrator cadre
// started (which has the text in its prompt already), and it never fails:
// a hook that errors would get in the way of every session.
func runHookOrchestrator(e *env) int {
	if inMember() || os.Getenv("CADRE_OFF") != "" || os.Getenv("CADRE_ORCHESTRATOR") != "" {
		return 0
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

// runHookSession is the hidden cadre hook session, the session start hook
// in every member's settings copy: when the member's session starts, or
// starts over after /clear or /compact, it records the conversation id it
// now has, so the next start resumes the right conversation. It writes
// only a record of a cadre cadre knows, for a name cadre makes, and it
// never fails.
func runHookSession(e *env) int {
	name, home := os.Getenv("CADRE_MEMBER"), os.Getenv("CADRE_HOME")
	if name == "" || home == "" {
		return 0
	}
	var own *cadres.Cadre
	list, _ := cadres.List()
	for i, c := range list {
		if c.Path == paths.Real(home) {
			own = &list[i]
		}
	}
	rt, err := runtime.Get(runtimeName())
	// Only a member's record: the orchestrator's is cadre's own, written
	// at launch, and a name that is no team and role of this cadre is no
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

// memberOf reports whether name is a member session name of cadre c:
// <cadre>-<team>-<role> or <cadre>-<team>-<project>-<role>, for a team
// folder and a role file that exist.
func memberOf(c cadres.Cadre, name string) bool {
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
