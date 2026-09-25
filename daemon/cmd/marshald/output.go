package main

import (
	"fmt"
	"io"
)

// say writes a line to the terminal. A failed write to a closed terminal has nowhere to be
// reported, so it is dropped on purpose.
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}
