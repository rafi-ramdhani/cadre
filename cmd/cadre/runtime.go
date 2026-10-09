//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/conf"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
	// The Claude Code adapter, the only runtime.
	_ "github.com/rafi-ramdhani/cadre/internal/runtime/claude"
)

// cutKeys are cadre.conf keys that 0.2.0 no longer reads, and what to do
// instead.
var cutKeys = []struct{ key, why string }{
	{"RUNTIME", "there is one runtime, with no choice to make"},
	{"ORCHESTRATOR_RUNTIME", "there is one runtime, with no choice to make"},
	{"ORCHESTRATOR_TMUX", "open the orchestrator in tmux with cadre --tmux"},
	{"ORCHESTRATOR_PERMISSION_MODE", "PERMISSION_MODE sets the orchestrator's mode too"},
}

// conf reads the cadre's cadre.conf, warning about lines it ignores and
// keys it no longer reads.
func (e *env) conf(r *cadres.Resolved) map[string]string {
	raw, _ := os.ReadFile(filepath.Join(r.Path, "cadre.conf"))
	values, warnings := conf.Parse(string(raw))
	for _, w := range warnings {
		fmt.Fprintln(e.stderr, "warning: "+w)
	}
	for _, k := range cutKeys {
		if _, set := values[k.key]; set {
			fmt.Fprintf(e.stderr, "warning: cadre.conf sets %s, which cadre no longer reads: %s\n", k.key, k.why)
		}
	}
	return values
}

// cadreRuntime is the runtime every session runs: Claude Code (a test
// build may name another, see runtime_cadretest.go).
func (e *env) cadreRuntime(r *cadres.Resolved) (runtime.Runtime, bool) {
	rt, err := runtime.Get(runtimeName())
	if err != nil {
		e.fail("%s", err)
		return nil, false
	}
	return rt, true
}

// places are the folders a runtime's fixed denies must name: ~/.cadre as
// resolved, the resolved cadre (registered or not, as with CADRE_HOME), and
// every known cadre.
func places(r *cadres.Resolved) runtime.Places {
	p := runtime.Places{Root: paths.Real(cadres.Root()), Cadres: []string{r.Path}}
	if list, err := cadres.List(); err == nil {
		for _, c := range list {
			if c.Path != r.Path {
				p.Cadres = append(p.Cadres, c.Path)
			}
		}
	}
	return p
}
