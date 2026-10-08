//go:build !windows && !cadretest

package main

// testTTY is false in release builds: only a real terminal counts.
// Unit tests set ttyForTests; the smoke test builds with -tags cadretest.
func testTTY() bool { return ttyForTests }

var ttyForTests bool
