//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rafi-ramdhani/cadrei/internal/cadreis"
	"github.com/rafi-ramdhani/cadrei/internal/conf"
	"github.com/rafi-ramdhani/cadrei/internal/framework"
	"github.com/rafi-ramdhani/cadrei/internal/paths"
	"github.com/rafi-ramdhani/cadrei/internal/runtime"
	// The Claude Code adapter, the only runtime.
	_ "github.com/rafi-ramdhani/cadrei/internal/runtime/claude"
)

// cutKeys are cadrei.conf keys that 0.2.0 no longer reads, and what to do
// instead.
var cutKeys = []struct{ key, why string }{
	{"RUNTIME", "there is one runtime, with no choice to make"},
	{"ORCHESTRATOR_RUNTIME", "there is one runtime, with no choice to make"},
	{"ORCHESTRATOR_TMUX", "open the orchestrator in tmux with cadrei --tmux"},
	{"ORCHESTRATOR_PERMISSION_MODE", "PERMISSION_MODE sets the orchestrator's mode too"},
}

// conf reads the cadrei's cadrei.conf, warning about lines it ignores and
// keys it no longer reads.
func (e *env) conf(r *cadreis.Resolved) map[string]string {
	raw, _ := os.ReadFile(filepath.Join(r.Path, "cadrei.conf"))
	values, warnings := conf.Parse(string(raw))
	for _, w := range warnings {
		fmt.Fprintln(e.stderr, "warning: "+w)
	}
	for _, k := range cutKeys {
		if _, set := values[k.key]; set {
			fmt.Fprintf(e.stderr, "warning: cadrei.conf sets %s, which cadrei no longer reads: %s\n", k.key, k.why)
		}
	}
	return values
}

// cadreiRuntime is the runtime every session runs: Claude Code (a test
// build may name another, see runtime_cadreitest.go).
func (e *env) cadreiRuntime(r *cadreis.Resolved) (runtime.Runtime, bool) {
	rt, err := runtime.Get(runtimeName())
	if err != nil {
		e.fail("%s", err)
		return nil, false
	}
	return rt, true
}

// places are the folders a runtime's fixed denies must name: ~/.cadrei as
// resolved, the resolved cadrei (registered or not, as with CADREI_HOME), and
// every known cadrei.
func places(r *cadreis.Resolved) runtime.Places {
	p := runtime.Places{Root: paths.Real(cadreis.Root()), Cadreis: []string{r.Path}, Binary: framework.Binary()}
	if list, err := cadreis.List(); err == nil {
		for _, c := range list {
			if c.Path != r.Path {
				p.Cadreis = append(p.Cadreis, c.Path)
			}
		}
	}
	return p
}
