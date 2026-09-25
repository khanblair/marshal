//go:build unix

package pty

import (
	"fmt"

	gopty "github.com/aymanbagabas/go-pty"
)

// defaultLineEnd is the line end that Send types. A Unix terminal turns it into what a program
// reading lines expects.
const defaultLineEnd = "\n"

// closeOnPolite says whether the polite stop closes the terminal. On Unix a signal is enough, and
// the terminal stays open so the last words of the program are still read.
const closeOnPolite = false

// releaseChildEnd closes our copy of the terminal's child end, once the child has its own. While
// we hold it, the read side never learns that the program is gone.
func releaseChildEnd(p gopty.Pty) error {
	unixPty, ok := p.(gopty.UnixPty)
	if !ok {
		return nil
	}
	if err := unixPty.Slave().Close(); err != nil {
		return fmt.Errorf("close the child end of the terminal: %w", err)
	}
	return nil
}
