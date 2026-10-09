//go:build !windows && cadretest

package main

import (
	"os"

	"github.com/rafi-ramdhani/cadre/internal/runtime"
	// The test-only fake runtime, never in a release binary.
	_ "github.com/rafi-ramdhani/cadre/internal/runtime/fake"
)

// runtimeName is the runtime sessions run. A test build can name another
// with CADRE_TEST_RUNTIME (the fake), to check that what runs is what the
// runtime built.
func runtimeName() string {
	if n := os.Getenv("CADRE_TEST_RUNTIME"); n != "" {
		return n
	}
	return runtime.Default()
}
