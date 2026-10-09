// Package runtime is the boundary between cadre and the agent CLI its
// sessions run. Cadre's own code (sessions, ls, allow, trust, health)
// talks to a Runtime; only an adapter, internal/runtime/claude, knows a
// CLI's flags, files, hooks and rule grammar. Claude Code is the only
// runtime, with no user-facing choice; the boundary keeps its specifics in
// one place, and lets tests run a fake.
package runtime

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// MessagingKind is how a runtime's sessions send each other messages.
type MessagingKind int

const (
	NoMessaging MessagingKind = iota // cannot be a persona or the orchestrator
	Native                           // the CLI's own cross-session messaging
)

// Capabilities say what a runtime supports, so a runtime that cannot run
// cadre's sessions safely is refused instead of failing in odd ways.
type Capabilities struct {
	FixedDenies      bool          // it enforces cadre's fixed denies; without it, personas are refused
	PermissionModes  []string      // the PERMISSION_MODE values it understands
	Resume           bool          // resuming a conversation
	AssignSessionID  bool          // starting with a session id cadre chose
	Trust            bool          // marking project folders as trusted
	Instructions     bool          // can hold the orchestrator's instructions
	OrchestratorHook bool          // can start sessions as the orchestrator
	Messaging        MessagingKind // how its sessions reach each other
}

// Role is what a session is for.
type Role int

const (
	Persona Role = iota
	Orchestrator
)

// LaunchSpec is what cadre knows about a session it starts.
type LaunchSpec struct {
	Role       Role
	Name       string // the session's name, its messaging address
	Cadre      string // the cadre's physical folder
	WorkDir    string
	Mode       string // cadre's PERMISSION_MODE
	PromptFile string // instructions to add to the session's own
	Grants     string // the runtime's grants artifact (from Permissions().Prepare), or ""
}

// Command is what cadre runs: in tmux for a persona, or as a child process.
type Command struct {
	Argv []string
	Env  []string // KEY=value, added to cadre's environment
	Dir  string
}

// Install is a runtime found on this machine.
type Install struct {
	Path    string
	Version string
}

// Folder is a project folder: its name and its path as cadre spells it.
type Folder struct {
	Name string
	Dir  string
}

// TrustResult is what happened to one folder's trust.
type TrustResult struct {
	Name   string
	State  string // "trusted", "already", "untrusted", "refused" or "skipped"
	Reason string
}

// Places are the physical folders a runtime's fixed denies must name:
// ~/.cadre as resolved, and every known cadre.
type Places struct {
	Root   string
	Cadres []string
}

// Prepared is what Permissions().Prepare made for a start.
type Prepared struct {
	Grants   string   // the artifact for LaunchSpec.Grants, or "" when the persona starts without
	Notes    []string // printed on stdout (a file was created)
	Warnings []string // printed on stderr
}

// Grant is one entry of a cadre's grants, as cadre allow list shows it.
type Grant struct {
	Kind  string // "rule", or "auto" for a plain-English allowance
	Entry string
	Once  bool      // a one-time grant, to be removed when its task is done
	Added time.Time // when a one-time grant was added
	Wide  bool      // the entry has a wildcard
}

// ErrGranted is returned when a grant is already there.
var ErrGranted = errors.New("already granted")

// ErrNoOnce is returned when there are no one-time grants to remove.
var ErrNoOnce = errors.New("there are no one-time grants")

// GrantStore is a cadre's grants, opened for reading and changing.
type GrantStore interface {
	List() []Grant
	Stale() []string // one-time records whose grant is gone
	HasOnce() bool
	Add(kind, entry string, once bool) error
	// Remove takes out a grant by entry or number, or every one-time grant
	// for "--once". It returns what it removed and the stale records it
	// dropped.
	Remove(target string) (removed, stale []string, err error)
	Files() []string // the files to commit, inside the cadre
}

// PermissionOps turn cadre's grants into what the runtime enforces.
type PermissionOps interface {
	// Open reads the cadre's grants, creating the file when create is set;
	// an error says why the file cannot be used.
	Open(cadre string, create bool) (GrantStore, error)
	// Unchanged reports whether the grants file is as cadre last wrote it,
	// recording it when it is what cadre last committed.
	Unchanged(cadre string) bool
	// Record remembers the grants file as cadre wrote it.
	Record(cadre string)
	// BuiltIn says what every persona gets that cadre allow list does not show.
	BuiltIn() string
	// Validate checks a grant before it is stored: a rule, or a
	// plain-English entry when auto is set. It returns a warning, or an
	// error saying why it is refused.
	Validate(cadre string, known []string, rule string, auto bool) (warning string, err error)
	// Prepare makes sure the cadre's grants file exists and writes what a
	// persona starts with, with the fixed denies added.
	Prepare(cadre string, places Places) Prepared
	// GrantsFile is where the cadre's grants are stored.
	GrantsFile(cadre string) string
}

// TrustOps mark project folders as trusted in the runtime.
type TrustOps interface {
	Mark(folders []Folder) (results []TrustResult, note string)
	Unmark(folders []Folder) (results []TrustResult, note string)
	// Protected lists folders the runtime loads code or settings from;
	// projects must never live inside them.
	Protected() []string
}

// Runtime is an agent CLI cadre can run sessions with.
type Runtime interface {
	Name() string
	Title() string // the product's name, for messages: "Claude Code"
	Caps() Capabilities
	Detect() (Install, error)
	// Launch builds the command for a session; the caller runs it.
	Launch(LaunchSpec) (Command, error)
	// BuildDir is where cadre writes a cadre's generated prompts and
	// grants artifacts for this runtime.
	BuildDir(cadre string) string
	Permissions() PermissionOps
	Trust() TrustOps
}

var (
	registry    = map[string]Runtime{}
	defaultName string
)

// Register adds a runtime. Adapters call it from init.
func Register(r Runtime) { registry[r.Name()] = r }

// RegisterDefault adds the runtime used when nothing names one.
func RegisterDefault(r Runtime) {
	Register(r)
	defaultName = r.Name()
}

// Default is the name of the runtime used when nothing names one.
func Default() string { return defaultName }

// Supported lists the runtimes this build accepts, sorted.
func Supported() []string {
	var out []string
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Get returns a runtime by name, refusing one this build does not have.
func Get(name string) (Runtime, error) {
	if r, ok := registry[name]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("runtime %s is not supported yet (supported: %s)", name, strings.Join(Supported(), ", "))
}

// CanOrchestrate refuses a runtime that cannot be the orchestrator:
// it needs a place for the orchestrator's instructions, a way to start a
// session as the orchestrator, and messaging to reach the personas.
func CanOrchestrate(r Runtime) error {
	caps := r.Caps()
	if !caps.Instructions || !caps.OrchestratorHook || caps.Messaging == NoMessaging {
		return fmt.Errorf("runtime %s cannot be the orchestrator (it needs instructions, an orchestrator start and messaging)", r.Name())
	}
	return nil
}

// Usable refuses a runtime that cannot run a persona safely or reach the
// orchestrator: one without the fixed denies, or without messaging.
func Usable(r Runtime, mode string) error {
	caps := r.Caps()
	if !caps.FixedDenies {
		return fmt.Errorf("runtime %s cannot enforce cadre's fixed denies, so its personas are refused", r.Name())
	}
	if caps.Messaging == NoMessaging {
		return fmt.Errorf("runtime %s has no way to message the orchestrator, so it cannot run personas", r.Name())
	}
	for _, m := range caps.PermissionModes {
		if m == mode {
			return nil
		}
	}
	return fmt.Errorf("runtime %s has no permission mode %s (it has: %s)", r.Name(), mode, strings.Join(caps.PermissionModes, ", "))
}
