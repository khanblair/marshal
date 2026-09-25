//go:build windows

package pty

import (
	gopty "github.com/aymanbagabas/go-pty"
)

// defaultLineEnd is the line end that Send types. The console expects the key that Enter sends.
const defaultLineEnd = "\r"

// closeOnPolite says whether the polite stop closes the terminal. Windows has no signal to ask a
// console program to end, and closing the pseudo console is what the console itself does when its
// window closes.
const closeOnPolite = true

// releaseChildEnd does nothing here: the pseudo console has no child end to hand over.
func releaseChildEnd(gopty.Pty) error { return nil }
