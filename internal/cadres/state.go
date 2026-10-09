package cadres

import (
	"encoding/json"
	"os"

	"github.com/rafi-ramdhani/cadre/internal/fsx"
)

// State is ~/.cadre/config/state.json: answers and versions cadre
// remembers between runs (N.1), as string keys and values.

// State keys.
const (
	// CheckedVersion is the version the last full health check ran for.
	CheckedVersion = "checked_version"
	// HookKept is "<program> -> <binary>": the user kept the orchestrator
	// hook on program when asked to switch it to binary.
	HookKept = "hook_kept"
)

// GetState returns a remembered value, or "".
func GetState(key string) string { return readState()[key] }

func readState() map[string]string {
	m := map[string]string{}
	raw, err := os.ReadFile(Config("state.json"))
	if err == nil {
		json.Unmarshal(raw, &m)
	}
	return m
}

// SetState remembers a value; "" forgets it.
func SetState(key, value string) error {
	l, err := lockConfig()
	if err != nil {
		return err
	}
	defer l.Release()
	m := readState()
	if value == "" {
		delete(m, key)
	} else {
		m[key] = value
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return fsx.WriteFile(Config("state.json"), append(data, '\n'), 0o600)
}
