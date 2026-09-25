package catalog

import (
	"strconv"
	"strings"
)

// maxVersionPart keeps a number in a version to nine digits, so it always fits an int and a
// long string of digits is not taken for a version.
const maxVersionPart = 9

// semverParts is major, minor, and patch: a version has exactly three dot-separated numbers.
const semverParts = 3

// Semver is a version of the form major.minor.patch, with an optional pre-release tag. Agents
// print their versions in different ways ("2.1.282 (Claude Code)", "0.35.1", "codex-cli 0.42.0"),
// and this is the small reader that finds the number in all of them.
type Semver struct {
	Major, Minor, Patch int
	// Pre is the pre-release tag without its leading dash, such as "preview.1". It is empty for
	// a release.
	Pre string
}

// ParseSemver finds the first version in the text: the first word that reads as major.minor.patch,
// with an optional leading "v", pre-release tag, and build tag. It returns false when the text
// holds none.
func ParseSemver(text string) (Semver, bool) {
	for _, word := range strings.Fields(text) {
		if v, ok := parseWord(word); ok {
			return v, true
		}
	}
	return Semver{}, false
}

// parseWord reads one word as a version.
func parseWord(word string) (Semver, bool) {
	word = strings.TrimLeft(word, "v(")
	word = strings.TrimRight(word, ",;)")
	// The build tag after a plus sign does not change which version it is.
	word, _, _ = strings.Cut(word, "+")
	core, pre, _ := strings.Cut(word, "-")
	parts := strings.Split(core, ".")
	if len(parts) != semverParts {
		return Semver{}, false
	}
	var nums [semverParts]int
	for i, part := range parts {
		n, ok := parsePart(part)
		if !ok {
			return Semver{}, false
		}
		nums[i] = n
	}
	return Semver{Major: nums[0], Minor: nums[1], Patch: nums[2], Pre: pre}, true
}

// parsePart reads one number of a version.
func parsePart(part string) (int, bool) {
	if part == "" || len(part) > maxVersionPart {
		return 0, false
	}
	for _, r := range part {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(part)
	return n, err == nil
}

// String returns the version as text, for example "2.1.282" or "0.36.0-preview.1".
func (v Semver) String() string {
	text := strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
	if v.Pre != "" {
		text += "-" + v.Pre
	}
	return text
}

// AtLeast says whether the version is the given release or a later one. A pre-release of a
// version counts as coming before that version.
func (v Semver) AtLeast(major, minor, patch int) bool {
	switch {
	case v.Major != major:
		return v.Major > major
	case v.Minor != minor:
		return v.Minor > minor
	case v.Patch != patch:
		return v.Patch > patch
	}
	return v.Pre == ""
}
