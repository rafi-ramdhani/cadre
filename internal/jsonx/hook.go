//go:build !cadretest

package jsonx

// raceHook is nil in release builds: they never run a command named by
// the environment. Unit tests set it directly; the smoke test builds with
// the cadretest tag (hook_cadretest.go).
var raceHook func(attempt int, path string)
