package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// ToolInputFingerprint compares tool arguments without retaining their content.
// Canonical JSON keeps object key order from making the same edit look different.
func ToolInputFingerprint(input json.RawMessage) string {
	var value any
	if len(input) == 0 || json.Unmarshal(input, &value) != nil {
		return ""
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
