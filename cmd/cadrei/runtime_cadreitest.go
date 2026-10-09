//go:build !windows && cadreitest

package main

import (
	"os"
	"time"

	"github.com/rafi-ramdhani/cadrei/internal/runtime"
	// The test-only fake runtime, never in a release binary.
	_ "github.com/rafi-ramdhani/cadrei/internal/runtime/fake"
)

// runtimeName is the runtime sessions run. A test build can name another
// with CADREI_TEST_RUNTIME (the fake), to check that what runs is what the
// runtime built.
func runtimeName() string {
	if n := os.Getenv("CADREI_TEST_RUNTIME"); n != "" {
		return n
	}
	return runtime.Default()
}

// upWait is how long up waits before it checks that a session started. A
// test build can give the smoke test more time on a busy machine with
// CADREI_TEST_UP_WAIT (a Go duration).
func upWait() time.Duration {
	d, _ := time.ParseDuration(os.Getenv("CADREI_TEST_UP_WAIT"))
	return d
}
