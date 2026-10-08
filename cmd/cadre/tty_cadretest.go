//go:build !windows && cadretest

package main

import "os"

// testTTY, in a binary built with -tags cadretest for the smoke test,
// lets CADRE_TEST_TTY=1 make answers come from stdin.
func testTTY() bool { return ttyForTests || os.Getenv("CADRE_TEST_TTY") == "1" }

var ttyForTests bool
