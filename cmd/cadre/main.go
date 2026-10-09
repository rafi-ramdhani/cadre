//go:build !windows

// Command cadre starts, stops and lists persona sessions: Claude Code
// sessions in tmux that an orchestrator session leads by name.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
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
	lines  *bufio.Reader // stdin, read by ask, one reader for every question
	eof    bool          // the input ended while cadre asked
}

func (e *env) say(format string, a ...any) { fmt.Fprintf(e.stdout, format+"\n", a...) }

// fail prints "cadre: <message>" on stderr and returns exit code 1.
func (e *env) fail(format string, a ...any) int {
	fmt.Fprintf(e.stderr, "cadre: "+format+"\n", a...)
	return 1
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	e := &env{stdin: stdin, stdout: stdout, stderr: stderr}
	c, rest, err := lookup(args)
	if err != nil {
		return e.fail("%s", err)
	}
	e.args = rest
	return c.run(e)
}
