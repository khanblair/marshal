//go:build linux || darwin

package main

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"unsafe"
)

// winsize is the layout that the terminal size call fills in.
type winsize struct {
	rows, cols, x, y uint16
}

// terminalSize asks the terminal for its size with the standard library only.
func terminalSize() (cols, rows int, err error) {
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return 0, 0, errno
	}
	if ws.cols == 0 || ws.rows == 0 {
		return 0, 0, errors.New("the terminal reports no size")
	}
	return int(ws.cols), int(ws.rows), nil
}

// notifySizeChange sends on ch each time the terminal changes size.
func notifySizeChange(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGWINCH)
}
