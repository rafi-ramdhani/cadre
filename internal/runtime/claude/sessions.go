package claude

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/rafi-ramdhani/cadre/internal/runtime"
)

func (Claude) Sessions() runtime.SessionOps { return sessions{} }

type sessions struct{}

// uuidRule is the form of a Claude Code session id.
var uuidRule = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// NewID is a random (version 4) UUID, which --session-id takes.
func (sessions) NewID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Exists looks for the conversation's transcript, which Claude Code keeps
// in <config>/projects/<folder>/<id>.jsonl.
func (sessions) Exists(id string) bool {
	if !uuidRule.MatchString(id) {
		return false
	}
	found, _ := filepath.Glob(filepath.Join(ConfigDir(), "projects", "*", id+".jsonl"))
	for _, f := range found {
		if st, err := os.Stat(f); err == nil && st.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// FromHook reads session_id from the SessionStart hook's JSON input.
func (sessions) FromHook(input io.Reader) string {
	var in struct {
		SessionID string `json:"session_id"`
	}
	if json.NewDecoder(io.LimitReader(input, 1<<20)).Decode(&in) != nil || !uuidRule.MatchString(in.SessionID) {
		return ""
	}
	return in.SessionID
}

// SessionHook is the session start hook a member's settings copy carries,
// so its recorded conversation id follows /clear and /compact.
func SessionHook(binary string) string { return quote(binary) + " hook session" }
