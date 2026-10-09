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

// conf reads the cadre's cadre.conf, warning about lines it ignores.
func (e *env) conf(r *cadres.Resolved) map[string]string {
	raw, _ := os.ReadFile(filepath.Join(r.Path, "cadre.conf"))
	values, warnings := conf.Parse(string(raw))
	for _, w := range warnings {
		fmt.Fprintln(e.stderr, "warning: "+w)
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
