//go:build !windows

package main

import (
	"testing"

	"github.com/rafi-ramdhani/cadre/internal/testguard"
)

func TestMain(m *testing.M) { testguard.Main(m) }
