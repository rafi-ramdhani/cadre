//go:build !windows && !cadretest

package main

import "github.com/rafi-ramdhani/cadre/internal/runtime"

// runtimeName is the runtime sessions run: always the default, Claude Code.
func runtimeName() string { return runtime.Default() }
