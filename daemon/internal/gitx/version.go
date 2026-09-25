package gitx

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// The oldest Git that Marshal supports.
const (
	minMajor = 2
	minMinor = 38
)

// MinVersion is the oldest Git that has what Marshal needs: worktrees, sparse checkout, and
// `git merge-tree --write-tree`.
func MinVersion() Version { return Version{Major: minMajor, Minor: minMinor} }

// ErrTooOld means the installed Git is older than MinVersion.
var ErrTooOld = errors.New("Git is too old")

// Version is a Git version. Only the first two numbers matter here.
type Version struct {
	Major, Minor int
}

func (v Version) String() string { return fmt.Sprintf("%d.%d", v.Major, v.Minor) }

// AtLeast reports whether v is the same as or newer than other.
func (v Version) AtLeast(other Version) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	return v.Minor >= other.Minor
}

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)`)

// ParseVersion reads the output of `git --version`, for example "git version 2.53.0" or
// "git version 2.39.5 (Apple Git-154)".
func ParseVersion(output string) (Version, error) {
	m := versionPattern.FindStringSubmatch(output)
	if m == nil {
		return Version{}, fmt.Errorf("could not read a Git version from %q", output)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return Version{Major: major, Minor: minor}, nil
}

// Version asks the installed Git for its version.
func (g *Git) Version(ctx context.Context) (Version, error) {
	out, err := g.Run(ctx, "", "--version")
	if err != nil {
		return Version{}, err
	}
	return ParseVersion(out)
}

// CheckVersion returns an error with a plain message when Git is missing or older than MinVersion.
func (g *Git) CheckVersion(ctx context.Context) error {
	v, err := g.Version(ctx)
	if err != nil {
		return err
	}
	if !v.AtLeast(MinVersion()) {
		return fmt.Errorf("%w: Git %s or newer is needed, and this machine has %s", ErrTooOld, MinVersion(), v)
	}
	return nil
}
