package platform_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/platform"
)

// TestMain covers every test in this package's test binary, internal and external.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestAcquireLockRefusesASecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marshald.lock")
	first, err := platform.AcquireLock(path)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	defer func() { _ = first.Release() }()

	if _, err := platform.AcquireLock(path); !errors.Is(err, platform.ErrAlreadyRunning) {
		t.Fatalf("second AcquireLock = %v, want an error wrapping ErrAlreadyRunning", err)
	}
}

func TestAcquireLockSucceedsAfterRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marshald.lock")
	first, err := platform.AcquireLock(path)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	second, err := platform.AcquireLock(path)
	if err != nil {
		t.Fatalf("third AcquireLock after release: %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestAcquireLockIsPerDataFolder(t *testing.T) {
	root := t.TempDir()
	devPath := filepath.Join(root, "Marshal-dev", "marshald.lock")
	normalPath := filepath.Join(root, "Marshal", "marshald.lock")

	dev, err := platform.AcquireLock(devPath)
	if err != nil {
		t.Fatalf("AcquireLock(dev): %v", err)
	}
	defer func() { _ = dev.Release() }()

	normal, err := platform.AcquireLock(normalPath)
	if err != nil {
		t.Fatalf("AcquireLock(normal) while dev is held: %v", err)
	}
	defer func() { _ = normal.Release() }()
}

func TestAcquireLockMakesTheFolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "marshald.lock")
	lock, err := platform.AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer func() { _ = lock.Release() }()
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Errorf("the lock folder was not created: %v", err)
	}
}

func TestAcquireLockFileContentIsReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marshald.lock")
	lock, err := platform.AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer func() { _ = lock.Release() }()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the lock file: %v", err)
	}
	got := strings.TrimSpace(string(data))
	want := strconv.Itoa(os.Getpid())
	if got != want {
		t.Errorf("lock file content = %q, want the process id %q", got, want)
	}
}

// TestAcquireLockFailureLeavesTheHolderPIDIntact proves a losing second acquire never truncates
// the file the first holder already wrote: it must open without O_TRUNC and only ever write after
// the lock itself is taken.
func TestAcquireLockFailureLeavesTheHolderPIDIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marshald.lock")
	first, err := platform.AcquireLock(path)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	defer func() { _ = first.Release() }()

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	if _, err := platform.AcquireLock(path); !errors.Is(err, platform.ErrAlreadyRunning) {
		t.Fatalf("second AcquireLock = %v, want ErrAlreadyRunning", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("the losing acquire changed the lock file: before %q, after %q", before, after)
	}
}

func TestReleaseOnNilLockIsSafe(t *testing.T) {
	var lock *platform.Lock
	if err := lock.Release(); err != nil {
		t.Errorf("Release on a nil lock = %v, want nil", err)
	}
}
