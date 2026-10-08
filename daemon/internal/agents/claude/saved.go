package claude

import (
	"os"
	"path/filepath"
)

// Saved says whether Claude Code has saved a conversation for a session id. It keeps each one as
// <id>.jsonl in a folder per project, and a session with no message has no file. When the answer
// cannot be found out, it says yes, and the resume is tried as it was before.
func Saved(sessionID string) bool {
	root := os.Getenv("CLAUDE_CONFIG_DIR")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return true
		}
		root = filepath.Join(home, ".claude")
	}
	found, err := filepath.Glob(filepath.Join(root, "projects", "*", sessionID+".jsonl"))
	return err != nil || len(found) > 0
}
