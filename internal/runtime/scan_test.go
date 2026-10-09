package runtime

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Claude Code specifics that only internal/runtime/claude may name: its
// command, flags, folders and files, hook events and
// settings keys.
var (
	exact = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "Notification", "Stop",
		"permissions", "autoMode"}
	inside = []string{"--permission-mode", "--append-system-prompt-file", "--settings", "--session-id", "--resume",
		".claude", "CLAUDE_CONFIG_DIR", "hasTrustDialogAccepted", "soft_deny", "$defaults", "orchestrator-hook.sh"}
)

// prose are the only string literals outside the adapter that may name
// Claude Code: text about the product, not its command or files.
var prose = map[string]bool{
	"cadrei: a team of Claude Code sessions you lead from one conversation": true,
}

// TestNoClaudeOutsideTheAdapter scans the string literals of every Go
// file outside internal/runtime/claude (tests, the test-only fake and
// testguard, which only tests import, excepted) for Claude Code specifics.
func TestNoClaudeOutsideTheAdapter(t *testing.T) {
	root := filepath.Join("..", "..")
	var found []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			switch rel {
			case ".git", "internal/runtime/claude", "internal/runtime/fake", "internal/testguard", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if _, ok := n.(*ast.ImportSpec); ok {
				return false // an import path is not a string the program uses
			}
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if strings.Contains(strings.ToLower(s), "claude") && !prose[strings.TrimSuffix(s, "\n")] {
				found = append(found, fset.Position(lit.Pos()).String()+": "+lit.Value)
				return true
			}
			for _, x := range exact {
				if s == x {
					found = append(found, fset.Position(lit.Pos()).String()+": "+lit.Value)
				}
			}
			for _, x := range inside {
				if strings.Contains(s, x) {
					found = append(found, fset.Position(lit.Pos()).String()+": "+lit.Value)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range found {
		t.Errorf("Claude Code specific outside internal/runtime/claude: %s", f)
	}
}
