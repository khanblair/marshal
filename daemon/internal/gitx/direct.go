package gitx

import (
	"crypto/sha256"
	"fmt"
)

// DirectBaseRef keeps the folder snapshot taken when a card starts. Hashing the opaque card id
// makes the ref safe even when a caller supplies an id containing Git revision syntax.
func DirectBaseRef(cardID string) string { return directRef(cardID, "base") }

// DirectResultRef keeps the card's latest finished turn, independently of the shared branch.
func DirectResultRef(cardID string) string { return directRef(cardID, "result") }

func directRef(cardID, kind string) string {
	return fmt.Sprintf("refs/marshal/direct/%x/%s", sha256.Sum256([]byte(cardID)), kind)
}
