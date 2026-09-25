package gitx

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// dirMode is the permission of a folder that is put back after a clone is discarded.
const dirMode = 0o755

// relation says how one path stands to another.
type relation int

const (
	unrelated relation = iota
	sameFolder
	insideFolder
)

// checkAbsolute makes sure a path is absolute and has no ".." segment, and returns it cleaned. The
// segments are looked for before cleaning, because cleaning an absolute path quietly folds
// "/data/worktrees/../../etc" into "/etc", which hides what was asked for.
func checkAbsolute(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", newOpError(ErrBadPath, "the path must be a full path", nil)
	}
	for _, part := range strings.FieldsFunc(path, isSeparator) {
		if part == ".." {
			return "", newOpError(ErrBadPath, "the path must not contain \"..\"", nil)
		}
	}
	return filepath.Clean(path), nil
}

func isSeparator(r rune) bool { return r == '/' || r == filepath.Separator }

// resolveLoose follows symbolic links in as much of a path as exists, and keeps the rest as it
// is. A worktree that is about to be created has no folder yet, but its parent may be a link.
func resolveLoose(path string) string {
	path = filepath.Clean(path)
	var rest []string
	for cur := path; ; {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(rest) - 1; i >= 0; i-- {
				real = filepath.Join(real, rest[i])
			}
			return real
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return path
		}
		rest = append(rest, filepath.Base(cur))
		cur = parent
	}
}

// sameFolderAs reports whether two paths lead to the same folder. The file system decides, so
// links, trailing separators, and upper and lower case on case-insensitive volumes all work. When
// a folder does not exist, the parts that do exist are resolved and the rest is compared by name,
// ignoring case on Windows.
func sameFolderAs(a, b string) bool {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	if errA == nil && errB == nil {
		return os.SameFile(infoA, infoB)
	}
	if errA == nil || errB == nil {
		return false // one is a real folder and the other is not there
	}
	a, b = resolveLoose(a), resolveLoose(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// relationOf says whether child is the same folder as parent, is inside it, or is neither. Neither
// path has to exist.
func relationOf(parent, child string) relation {
	parent, child = resolveLoose(parent), resolveLoose(child)
	for cur, depth := child, 0; ; depth++ {
		if sameFolderAs(parent, cur) {
			if depth == 0 {
				return sameFolder
			}
			return insideFolder
		}
		up := filepath.Dir(cur)
		if up == cur {
			return unrelated
		}
		cur = up
	}
}

// errFolderNotEmpty is what emptyOrMissing says about a folder with something in it. The callers
// turn it into their own error, because a full folder means a different thing for a worktree and
// for a clone.
var errFolderNotEmpty = errors.New("the folder is not empty")

// emptyOrMissing checks a place where something new will be created. A missing path and an empty
// folder are fine. A file is an ErrBadPath, and a folder with anything in it is errFolderNotEmpty.
func emptyOrMissing(path string) (existed bool, err error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, newOpError(ErrBadPath, "the path cannot be read", err)
	}
	if !info.IsDir() {
		return true, newOpError(ErrBadPath, "there is a file at that path", nil)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return true, newOpError(ErrBadPath, "the folder cannot be read", err)
	}
	if len(entries) > 0 {
		return true, errFolderNotEmpty
	}
	return true, nil
}
