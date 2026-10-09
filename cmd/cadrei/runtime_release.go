//go:build !windows && !cadreitest

package main

import (
	"time"

	"github.com/rafi-ramdhani/cadrei/internal/runtime"
)

// runtimeName is the runtime sessions run: always the default, Claude Code.
func runtimeName() string { return runtime.Default() }

// upWait is how long up waits before it checks that a session started:
// the default.
func upWait() time.Duration { return 0 }
