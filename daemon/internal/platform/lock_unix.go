//go:build unix

package platform

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// lockFile takes an exclusive, non-blocking advisory lock on the whole file with flock(2). The
// lock is held by the open file description, so it is released automatically when the process
// exits or is killed, even a hard kill, without any cleanup code running.
func lockFile(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) {
		return fmt.Errorf("%w: %w", ErrAlreadyRunning, err)
	}
	return fmt.Errorf("lock %s: %w", f.Name(), err)
}

// unlockFile gives up the flock taken by lockFile.
func unlockFile(f *os.File) error {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_UN); err != nil {
		return fmt.Errorf("unlock %s: %w", f.Name(), err)
	}
	return nil
}
