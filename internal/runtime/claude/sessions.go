package claude

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rafi-ramdhani/cadre/internal/paths"
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

// Exists looks for the conversation's transcript where Claude Code keeps
// it for the folder the session ran in, <config>/projects/<folder>/<id>.jsonl,
// the folder written with every character but letters and digits as "-":
// that is where claude --resume looks.
func (sessions) Exists(id, dir string) bool {
	if !uuidRule.MatchString(id) || dir == "" {
		return false
	}
	for _, d := range []string{dir, paths.Real(dir)} {
		f := filepath.Join(ConfigDir(), "projects", projectFolder(d), id+".jsonl")
		if st, err := os.Lstat(f); err == nil && st.Mode().IsRegular() {
			return true
		}
	}
	return false
}

var notAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// projectFolder is the name Claude Code gives a folder under projects/.
func projectFolder(dir string) string { return notAlnum.ReplaceAllString(dir, "-") }

// ResumeFailed recognizes claude's error for a --resume id it cannot find.
func (sessions) ResumeFailed(output string) bool {
	return strings.Contains(strings.ToLower(output), "no conversation found")
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
