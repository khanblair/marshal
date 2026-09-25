package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// minResetDepth keeps a reset away from the top of the disk: the path must sit at least this
// many folders down.
const minResetDepth = 3

// ErrUnsafeReset means a folder is not one that a dev reset may delete.
var ErrUnsafeReset = errors.New("this folder is not a dev data folder")

// CheckResettable says whether a dev reset may delete the folder. It refuses anything that is
// not an absolute path ending in the dev suffix, the home folder itself, and paths near the top of
// the disk, so a wrong setting can never delete more than a dev data folder.
func CheckResettable(path, home string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%w: %q is not an absolute path", ErrUnsafeReset, path)
	}
	clean := filepath.Clean(path)
	if home != "" && clean == filepath.Clean(home) {
		return fmt.Errorf("%w: %q is the home folder", ErrUnsafeReset, clean)
	}
	if !strings.HasSuffix(strings.ToLower(filepath.Base(clean)), devSuffix) {
		return fmt.Errorf("%w: %q does not end in %q", ErrUnsafeReset, clean, devSuffix)
	}
	parts := strings.Split(strings.Trim(filepath.ToSlash(clean), "/"), "/")
	if len(parts) < minResetDepth {
		return fmt.Errorf("%w: %q is too close to the top of the disk", ErrUnsafeReset, clean)
	}
	return nil
}

// ResetDevData deletes a dev data folder after CheckResettable agrees. A folder that does not
// exist is not an error.
func ResetDevData(path, home string) error {
	if err := CheckResettable(path, home); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("delete the dev data folder: %w", err)
	}
	return nil
}
