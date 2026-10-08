package settings

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/fsx"
	"github.com/rafi-ramdhani/cadre/internal/jsonx"
)

// Grant kinds.
const (
	Rule = "rule" // a permission rule in permissions.allow
	Auto = "auto" // a plain-English entry in autoMode.allow
)

// Grant is one entry of the file, as cadre allow list shows it.
type Grant struct {
	Kind  string
	Entry string
	Once  bool      // a one-time grant, to be removed when its task is done
	Added time.Time // when a one-time grant was added
}

// Grants is a settings file opened for reading and changing its grants,
// with its .once sidecar ("<epoch>\t<entry>" per one-time grant).
type Grants struct {
	path, oncePath string
	root           *jsonx.Value
	allow, auto    *jsonx.Value
	once           []onceRec
	// Stale holds one-time records whose grant is no longer in the file
	// (removed by hand, or a write cut short). They are dropped on save.
	Stale []string
}

type onceRec struct {
	added int64
	entry string
}

// OncePath is the sidecar of a settings file.
func OncePath(path string) string { return strings.TrimSuffix(path, ".json") + ".once" }

// OpenGrants reads the file at path, which must be valid.
func OpenGrants(path string) (*Grants, error) {
	root, err := Load(path)
	if err != nil {
		return nil, err
	}
	g := &Grants{path: path, oncePath: OncePath(path), root: root}
	perms := root.Get("permissions")
	if perms == nil {
		perms = jsonx.NewObject()
		root.Set("permissions", perms)
	}
	if g.allow = perms.Get("allow"); g.allow == nil {
		g.allow = strs()
		perms.Set("allow", g.allow)
	}
	mode := root.Get("autoMode")
	if mode == nil {
		mode = jsonx.NewObject()
		root.Set("autoMode", mode)
	}
	if g.auto = mode.Get("allow"); g.auto == nil {
		g.auto = strs(Defaults)
		mode.Set("allow", g.auto)
	}
	if raw, err := os.ReadFile(g.oncePath); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			t, e, ok := strings.Cut(line, "\t")
			if !ok || e == "" {
				continue
			}
			n, _ := strconv.ParseInt(t, 10, 64)
			if g.Has(e) {
				g.once = append(g.once, onceRec{n, e})
			} else {
				g.Stale = append(g.Stale, e)
			}
		}
	}
	return g, nil
}

func entriesOf(v *jsonx.Value) []string {
	out, _ := texts(v)
	return out
}

// List returns the grants: rules first, then autoMode entries (without
// "$defaults"), in file order.
func (g *Grants) List() []Grant {
	var out []Grant
	add := func(kind string, e string) {
		gr := Grant{Kind: kind, Entry: e}
		for _, o := range g.once {
			if o.entry == e {
				gr.Once, gr.Added = true, time.Unix(o.added, 0)
			}
		}
		out = append(out, gr)
	}
	for _, e := range entriesOf(g.allow) {
		add(Rule, e)
	}
	for _, e := range entriesOf(g.auto) {
		if e != Defaults {
			add(Auto, e)
		}
	}
	return out
}

// Has reports whether entry is granted, as a rule or an autoMode entry.
func (g *Grants) Has(entry string) bool {
	for _, gr := range g.List() {
		if gr.Entry == entry {
			return true
		}
	}
	return false
}

// HasOnce reports whether there are one-time grants.
func (g *Grants) HasOnce() bool { return len(g.once) > 0 }

// ErrGranted is returned by Add for a grant already in the file.
var ErrGranted = errors.New("already granted")

// Add grants entry of kind, as a one-time grant when once is set, and
// saves. An added one-time grant reaches the sidecar before the file, so a
// write cut short leaves at most a stale sidecar line.
func (g *Grants) Add(kind, entry string, once bool, now time.Time) error {
	for _, gr := range g.List() {
		if gr.Kind == kind && gr.Entry == entry {
			return ErrGranted
		}
	}
	list := g.allow
	if kind == Auto {
		list = g.auto
	}
	list.Items = append(list.Items, jsonx.String(entry))
	if once {
		g.once = append(g.once, onceRec{now.Unix(), entry})
	}
	return g.save(true)
}

// Removal is what Remove did.
type Removal struct {
	Removed []string // grants taken out
	Stale   []string // one-time records dropped because their grant was gone
}

// NoGrant is returned by Remove when the target names no grant.
type NoGrant string

func (e NoGrant) Error() string { return string(e) }

// ErrNoOnce is returned by Remove("--once") when there is nothing to do.
var ErrNoOnce = errors.New("there are no one-time grants")

// Remove takes out a grant named by its entry or its number in List, or
// every one-time grant for "--once", and saves. A removed grant leaves the
// file before the sidecar.
func (g *Grants) Remove(target string) (Removal, error) {
	var gone []string
	list := g.List()
	switch n, err := strconv.Atoi(target); {
	case target == "--once":
		for _, o := range g.once {
			gone = append(gone, o.entry)
		}
		if len(gone) == 0 {
			if len(g.Stale) == 0 {
				return Removal{}, ErrNoOnce
			}
			stale := g.Stale
			return Removal{Stale: stale}, g.save(false)
		}
	case err == nil && target != "" && strings.Trim(target, "0123456789") == "":
		if n < 1 || n > len(list) {
			return Removal{}, NoGrant(fmt.Sprintf("there is no grant number %d (see cadre allow list)", n))
		}
		gone = []string{list[n-1].Entry}
	case g.Has(target):
		gone = []string{target}
	default:
		return Removal{}, NoGrant(fmt.Sprintf("%s is not granted (see cadre allow list)", target))
	}
	for _, e := range gone {
		drop(g.allow, e)
		drop(g.auto, e)
	}
	kept := g.once[:0]
	for _, o := range g.once {
		if !contains(gone, o.entry) {
			kept = append(kept, o)
		}
	}
	g.once = kept
	return Removal{Removed: gone, Stale: g.Stale}, g.save(false)
}

// drop removes the first occurrence of entry from a list.
func drop(list *jsonx.Value, entry string) {
	for i, it := range list.Items {
		if s, ok := it.Text(); ok && s == entry {
			list.Items = append(list.Items[:i], list.Items[i+1:]...)
			return
		}
	}
}

// save writes the file and the sidecar, in the order that keeps a write
// cut short harmless.
func (g *Grants) save(sidecarFirst bool) error {
	side := func() error {
		var b strings.Builder
		for _, o := range g.once {
			fmt.Fprintf(&b, "%d\t%s\n", o.added, o.entry)
		}
		return fsx.WriteFile(g.oncePath, []byte(b.String()), 0o644)
	}
	if sidecarFirst {
		if err := side(); err != nil {
			return err
		}
	}
	if err := fsx.WriteFile(g.path, jsonx.Format(g.root, "  "), 0o644); err != nil {
		return err
	}
	if !sidecarFirst {
		return side()
	}
	g.Stale = nil
	return nil
}
