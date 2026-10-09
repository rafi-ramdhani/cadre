//go:build !windows && !cadreitest

package main

// testTTY is false in release builds: only a real terminal counts.
// Unit tests set ttyForTests; the smoke test builds with -tags cadreitest.
func testTTY() bool { return ttyForTests }

var ttyForTests bool

// hookAnywhere is false in release builds: the hook names only a binary
// that is safely placed. Unit tests, whose binaries live in temporary
// folders, set hookAnywhereForTests.
func hookAnywhere() bool { return hookAnywhereForTests }

var hookAnywhereForTests bool
