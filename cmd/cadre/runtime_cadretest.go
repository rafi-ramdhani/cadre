//go:build !windows && cadretest

package main

// The test-only fake runtime (section P.8), never in a release binary.
import _ "github.com/rafi-ramdhani/cadre/internal/runtime/fake"
