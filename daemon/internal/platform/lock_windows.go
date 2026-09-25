//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// lockRegionOffset is a byte far past any realistic lock file content. LockFileEx's lock is
// mandatory on Windows: it would block an ordinary read of a locked range from any handle, not
// only from another flock-style caller. Locking a range out past the pid text keeps that text at
// the start of the file readable through a plain read while the lock is held.
const lockRegionOffset = 1 << 30

// lockFile takes an exclusive, non-blocking lock on one byte at lockRegionOffset with
// LockFileEx. The lock is held by the file handle, so it is released automatically when the
// process exits or is killed, even a hard kill, without any cleanup code running.
func lockFile(f *os.File) error {
	overlapped := windows.Overlapped{Offset: lockRegionOffset}
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &overlapped)
	if err == nil {
		return nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return fmt.Errorf("%w: %w", ErrAlreadyRunning, err)
	}
	return fmt.Errorf("lock %s: %w", f.Name(), err)
}

// unlockFile gives up the lock taken by lockFile.
func unlockFile(f *os.File) error {
	overlapped := windows.Overlapped{Offset: lockRegionOffset}
	if err := windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlapped); err != nil {
		return fmt.Errorf("unlock %s: %w", f.Name(), err)
	}
	return nil
}
