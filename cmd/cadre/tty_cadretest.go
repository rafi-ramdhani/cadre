//go:build !windows && cadretest

package main

import "os"

// testTTY, in a binary built with -tags cadretest for the smoke test,
// lets CADRE_TEST_TTY=1 make answers come from stdin.
func testTTY() bool { return ttyForTests || os.Getenv("CADRE_TEST_TTY") == "1" }

var ttyForTests bool

// hookAnywhere, in a test build, lets CADRE_TEST_HOOK_ANYWHERE=1 name a
// binary in a temporary folder in the hook, for the smoke test.
func hookAnywhere() bool {
	return hookAnywhereForTests || os.Getenv("CADRE_TEST_HOOK_ANYWHERE") == "1"
}

var hookAnywhereForTests bool
