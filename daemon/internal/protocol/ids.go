package protocol

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	projectIDMinLength = 2
	projectIDMaxLength = 24
	// firstDuplicateNumber is the number a second project with the same id gets: api, api-2.
	firstDuplicateNumber = 2
	// fallbackProjectID is what SlugFromName returns when a name has nothing usable in it.
	fallbackProjectID = "project"
	cardKeySeparator  = "#"
	maxCardNumberBits = 31
)

// ValidProjectID reports whether id is a project id: lower case letters, digits, and hyphens,
// 2 to 24 characters, starting with a letter. A project id never changes after the project is
// created, and it is used in routes and folder names, so the rule is strict.
func ValidProjectID(id string) bool {
	if len(id) < projectIDMinLength || len(id) > projectIDMaxLength {
		return false
	}
	if !isLowerLetter(id[0]) {
		return false
	}
	for i := 1; i < len(id); i++ {
		if c := id[i]; !isSlugByte(c) && c != '-' {
			return false
		}
	}
	return true
}

func isLowerLetter(c byte) bool { return c >= 'a' && c <= 'z' }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isSlugByte(c byte) bool { return isLowerLetter(c) || isDigit(c) }

// SlugFromName turns a project name into a candidate project id: lower case, every run of other
// characters becomes one hyphen, hyphens at either end are dropped, and the result is cut to 24
// characters. A name with no usable letters, or one that would not start with a letter, gives
// "project". The caller makes the id unique with NumberedProjectID.
func SlugFromName(name string) string {
	var slug strings.Builder
	pendingHyphen := false
	for _, r := range strings.ToLower(name) {
		if r >= utf8.RuneSelf || !isSlugByte(byte(r)) {
			pendingHyphen = true
			continue
		}
		if pendingHyphen && slug.Len() > 0 {
			slug.WriteByte('-')
		}
		pendingHyphen = false
		slug.WriteByte(byte(r))
	}
	id := slug.String()
	if len(id) > projectIDMaxLength {
		// Cutting can leave a hyphen at the end, so drop it again.
		id = strings.TrimRight(id[:projectIDMaxLength], "-")
	}
	if !ValidProjectID(id) {
		return fallbackProjectID
	}
	return id
}

// NumberedProjectID adds "-n" to a project id so it is unique, for n of 2 or more. It shortens
// the id when needed so the result is still at most 24 characters. A smaller n returns id as is.
func NumberedProjectID(id string, n int) string {
	if n < firstDuplicateNumber {
		return id
	}
	suffix := "-" + strconv.Itoa(n)
	if room := projectIDMaxLength - len(suffix); len(id) > room {
		id = strings.TrimRight(id[:room], "-")
	}
	return id + suffix
}

// CardKey names a card by its project and its number in that project. Numbers start at 1 in
// every project, so the number alone is not unique. It is not the card's opaque id.
type CardKey struct {
	// ProjectID is the project's short id.
	ProjectID string `json:"projectId"`
	// Number is the card's number in its project, starting at 1.
	Number int `json:"number"`
}

// String formats the key as <projectId>#<number>, for example web-dashboard#12. A project id
// never contains a hash sign, so the last one always separates the number. Escape the hash sign
// when the key goes into a URL.
func (k CardKey) String() string {
	return k.ProjectID + cardKeySeparator + strconv.Itoa(k.Number)
}

// ParseCardKey reads a key written by CardKey.String. It is strict: the project id must be
// valid and the number must be a whole number of 1 or more, with no sign and no leading zero.
func ParseCardKey(text string) (CardKey, error) {
	project, number, found := cutLast(text, cardKeySeparator)
	if !found || !ValidProjectID(project) {
		return CardKey{}, fmt.Errorf("parse card key %q: want <project id>#<number>", text)
	}
	if number == "" || number[0] == '0' {
		return CardKey{}, fmt.Errorf("parse card key %q: the number must start at 1", text)
	}
	n, err := strconv.ParseUint(number, 10, maxCardNumberBits)
	if err != nil {
		return CardKey{}, fmt.Errorf("parse card key %q: %w", text, err)
	}
	return CardKey{ProjectID: project, Number: int(n)}, nil
}

// cutLast splits text around the last separator.
func cutLast(text, sep string) (before, after string, found bool) {
	i := strings.LastIndex(text, sep)
	if i < 0 {
		return text, "", false
	}
	return text[:i], text[i+len(sep):], true
}
