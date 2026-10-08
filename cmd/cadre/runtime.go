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
	// The Claude Code adapter, the only runtime in 0.2.0.
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

// cadreRuntime is the cadre's runtime for work that is not one persona's
// (trust, project folders): RUNTIME in cadre.conf, else claude.
func (e *env) cadreRuntime(r *cadres.Resolved) (runtime.Runtime, bool) {
	name := e.conf(r)["RUNTIME"]
	if name == "" {
		name = runtime.Default()
	}
	rt, err := runtime.Get(name)
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
