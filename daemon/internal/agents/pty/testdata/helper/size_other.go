//go:build !linux && !darwin

package main

import (
	"errors"
	"os"
)

// terminalSize is not available here: reading the size of a console without a library that is not
// in the standard library is not worth it for a test helper.
func terminalSize() (cols, rows int, err error) {
	return 0, 0, errors.New("the terminal size cannot be read on this platform")
}

// notifySizeChange does nothing here.
func notifySizeChange(chan<- os.Signal) {}
