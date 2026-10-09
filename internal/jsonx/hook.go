//go:build !cadreitest

package jsonx

// raceHook is nil in release builds: they never run a command named by
// the environment. Unit tests set it directly; the smoke test builds with
// the cadreitest tag (hook_cadreitest.go).
var raceHook func(attempt int, path string)
