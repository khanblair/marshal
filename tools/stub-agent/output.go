package main

import (
	"fmt"
	"io"
)

// say writes a line to a terminal stream. A failed write to a closed stream has nowhere to be
// reported, so it is dropped on purpose.
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}

// terminal is what the program reads from and writes to. Standard output carries the protocol
// once the agent runs, so messages for a person go to standard error.
type terminal struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// environment is what the program reads from the machine.
type environment struct {
	getenv  func(string) string
	tempDir string
}
