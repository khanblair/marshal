package platform

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

const (
	// lockDirMode matches cmd/marshald's own data folder mode, since the lock file lives in the
	// same folder.
	lockDirMode  = 0o700
	lockFileMode = 0o600
)

// ErrAlreadyRunning means a Marshal daemon already holds the single-instance lock for this data
// folder. It is a plain sentence a person would read, since callers may show it as is.
var ErrAlreadyRunning = errors.New("marshal is already running for this user")

// Lock is a held single-instance lock on a data folder.
type Lock struct {
	file *os.File
}

// AcquireLock takes the single-instance lock at path: a real, OS-held advisory lock on the file,
// not a PID-file staleness guess. The operating system releases it on its own if this process
// dies, including a hard kill, which a PID file cannot do safely. AcquireLock creates the
// containing folder if it does not exist yet. When the lock is already held, it returns an error
// that wraps ErrAlreadyRunning.
func AcquireLock(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), lockDirMode); err != nil {
		return nil, fmt.Errorf("make the lock folder: %w", err)
	}
	// Never open with O_TRUNC: a second process that loses the lock race must not wipe the file
	// content the first process already holds and has written.
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, lockFileMode)
	if err != nil {
		return nil, fmt.Errorf("open the lock file: %w", err)
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := writePID(file); err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, err
	}
	return &Lock{file: file}, nil
}

// writePID truncates the file first, so a stale pid from a crashed process is never read as
// meaningful, then writes the current process id for a person reading the file. The lock itself
// does not depend on this content.
func writePID(file *os.File) error {
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("clear the lock file: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind the lock file: %w", err)
	}
	if _, err := file.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		return fmt.Errorf("write the lock file: %w", err)
	}
	return nil
}

// Release gives up the lock. It is safe to call on a nil Lock.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unlockFile(l.file)
	if closeErr := l.file.Close(); closeErr != nil && err == nil {
		err = fmt.Errorf("close the lock file: %w", closeErr)
	}
	return err
}
