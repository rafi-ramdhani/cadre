package claude

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
	"github.com/rafi-ramdhani/cadre/internal/runtime/claude/settings"
)

// Open reads the cadre's member settings file. With create, a missing
// file is made (with no grants), recorded and committed.
func (p permissions) Open(cadre string, create bool) (runtime.GrantStore, error) {
	file := p.GrantsFile(cadre)
	if _, err := os.Stat(file); err != nil && create {
		os.MkdirAll(filepath.Dir(file), 0o755)
		if err := settings.Create(file); err != nil {
			return nil, err
		}
		settings.Record(file, hashFile())
		cadres.Commit(cadre, "Add the member settings file", settings.Rel)
	}
	g, err := settings.OpenGrants(file)
	if err != nil {
		return nil, err
	}
	return store{g}, nil
}

func (p permissions) Unchanged(cadre string) bool {
	return !settings.ChangedOutside(cadre, p.GrantsFile(cadre), hashFile())
}

func (p permissions) Record(cadre string) { settings.Record(p.GrantsFile(cadre), hashFile()) }

func (permissions) BuiltIn() string {
	return "Built in, not listed: deny rules and a soft_deny entry that keep members from changing these grants."
}

type store struct{ g *settings.Grants }

func (s store) List() []runtime.Grant {
	var out []runtime.Grant
	for _, g := range s.g.List() {
		out = append(out, runtime.Grant{Kind: g.Kind, Entry: g.Entry, Once: g.Once, Added: g.Added,
			Wide: g.Kind == settings.Rule && strings.Contains(g.Entry, "*")})
	}
	return out
}

func (s store) Stale() []string { return s.g.Stale }
func (s store) HasOnce() bool   { return s.g.HasOnce() }

func (s store) Add(kind, entry string, once bool) error {
	err := s.g.Add(kind, entry, once, time.Now())
	if errors.Is(err, settings.ErrGranted) {
		return runtime.ErrGranted
	}
	return err
}

func (s store) Remove(target string) ([]string, []string, error) {
	r, err := s.g.Remove(target)
	if errors.Is(err, settings.ErrNoOnce) {
		return nil, nil, runtime.ErrNoOnce
	}
	return r.Removed, r.Stale, err
}

func (s store) Files() []string {
	return []string{settings.Rel, strings.TrimSuffix(settings.Rel, ".json") + ".once"}
}
