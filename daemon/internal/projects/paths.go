package projects

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// expandHome turns a leading ~ into the home folder of the user who runs the daemon, which is
// where the repositories are. People type ~/code/app in the path field.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find the home folder: %w", err)
	}
	return filepath.Join(home, path[1:]), nil
}

// sameFolder reports whether two paths lead to the same folder, however they are spelled. The
// file system decides, so links, trailing separators, and case on a case-insensitive volume all
// count. A path that cannot be read is a different folder.
func sameFolder(a, b string) bool {
	infoA, err := os.Stat(a)
	if err != nil {
		return false
	}
	infoB, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(infoA, infoB)
}

// within reports whether path is the folder parent or somewhere inside it. It compares the
// spelling of two cleaned paths, so resolve links first with resolveExisting.
func within(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// resolveExisting follows the links in the part of a path that exists, and keeps the rest as it
// is. A folder that is about to be made can be compared with folders that exist this way.
func resolveExisting(path string) string {
	clean := filepath.Clean(path)
	rest := ""
	for current := clean; ; {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return clean
		}
		rest = filepath.Join(filepath.Base(current), rest)
		current = parent
	}
}

// folderExists reports whether path is a folder that can be looked at.
func folderExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
