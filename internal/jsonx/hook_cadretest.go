//go:build cadretest

package jsonx

import (
	"os"
	"os/exec"
	"strconv"
)

// raceHook, in a binary built with -tags cadretest for the smoke test,
// runs CADRE_TEST_JSON_EDIT_HOOK (with the attempt number and the path)
// between writing the new file and the check, as the bash version did.
var raceHook = func(attempt int, path string) {
	if hook := os.Getenv("CADRE_TEST_JSON_EDIT_HOOK"); hook != "" {
		exec.Command(hook, strconv.Itoa(attempt), path).Run()
	}
}
