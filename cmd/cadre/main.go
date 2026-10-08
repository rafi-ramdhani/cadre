// Command cadre starts, stops and lists persona sessions: Claude Code
// sessions in tmux that an orchestrator session leads by name.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
)

// version is stamped at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// env is what a command reads and writes; tests pass their own.
type env struct {
	args   []string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (e *env) say(format string, a ...any) { fmt.Fprintf(e.stdout, format+"\n", a...) }

// fail prints "cadre: <message>" on stderr and returns exit code 1.
func (e *env) fail(format string, a ...any) int {
	fmt.Fprintf(e.stderr, "cadre: "+format+"\n", a...)
	return 1
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	e := &env{stdin: stdin, stdout: stdout, stderr: stderr}
	if runtime.GOOS == "windows" {
		return e.fail("Windows is not supported (cadre needs tmux); use WSL 2")
	}
	c, rest, err := lookup(args)
	if err != nil {
		return e.fail("%s", err)
	}
	e.args = rest
	return c.run(e)
}
