package cadres

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/rafi-ramdhani/cadre/internal/fsx"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/registry"
)

// A cadre's places are where its projects are on this machine:
// ~/.cadre/config/places/<cadre>.json, a JSON object of project name to
// folder (~/... under the home folder, absolute otherwise). The registry
// travels with the cadre and holds no local paths; each machine keeps its
// own places.

// PlacesFile is the cadre's places file.
func PlacesFile(c Cadre) string { return filepath.Join(ConfigDir(), "places", c.Name+".json") }

// Places returns the cadre's places as written (~ not expanded).
func Places(c Cadre) map[string]string {
	m := map[string]string{}
	if raw, err := os.ReadFile(PlacesFile(c)); err == nil {
		json.Unmarshal(raw, &m)
	}
	return m
}

// Place is the folder of a project on this machine, or "" when it has
// none. A relative entry, which cadre never writes, is ignored.
func Place(c Cadre, project string) string {
	p := expandHome(Places(c)[project])
	if !filepath.IsAbs(p) {
		return ""
	}
	return filepath.Clean(p)
}

// SetPlace records where a project is on this machine; dir "" forgets it.
func SetPlace(c Cadre, project, dir string) error {
	l, err := lockConfig()
	if err != nil {
		return err
	}
	defer l.Release()
	m := Places(c)
	if dir == "" {
		delete(m, project)
	} else {
		m[project] = Tilde(paths.Real(dir))
	}
	if err := os.MkdirAll(filepath.Dir(PlacesFile(c)), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return fsx.WriteFile(PlacesFile(c), append(data, '\n'), 0o600)
}

// ProjectDir is where a cadre's project is on this machine, or "" when it
// has no place here.
func ProjectDir(c Cadre, e *registry.Entry) string { return Place(c, e.Name) }

// Where says how a project's folder is on this machine: "present",
// "not here" (no place recorded), "missing" (the folder is gone), or
// "drive" (it is on a drive that is not connected).
func Where(dir string) string {
	if dir == "" {
		return "not here"
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return "present"
	}
	if onMissingDrive(dir) {
		return "drive"
	}
	return "missing"
}

// onMissingDrive reports whether dir is on a removable or network drive
// whose mount point is not there: /Volumes/<drive> on macOS, /media,
// /run/media or /mnt on Linux.
func onMissingDrive(dir string) bool {
	parts := strings.Split(filepath.Clean(dir), string(filepath.Separator))
	// parts[0] is "" for an absolute path.
	var mount string
	switch {
	case len(parts) > 2 && (parts[1] == "Volumes" || parts[1] == "mnt"):
		mount = filepath.Join("/", parts[1], parts[2])
	case len(parts) > 3 && parts[1] == "media":
		mount = filepath.Join("/", parts[1], parts[2], parts[3])
	case len(parts) > 4 && parts[1] == "run" && parts[2] == "media":
		mount = filepath.Join("/", parts[1], parts[2], parts[3], parts[4])
	default:
		return false
	}
	_, err := os.Stat(mount)
	return err != nil
}
