package claude

import (
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// commonArgs builds the flags that a new session and a resumed one share. --flag=value is used
// throughout, rather than "--flag value", so that a model name or an instructions string that
// starts with "-" is never read as another option.
func (c Config) commonArgs(spec agents.StartSpec) ([]string, error) {
	mode, err := c.permissionMode(spec.PermissionMode)
	if err != nil {
		return nil, err
	}
	args := []string{
		"--print", "--input-format=stream-json", "--output-format=stream-json",
		"--include-partial-messages", "--verbose",
		// No permission-prompt tool is wired up in Phase 1 (see claudeSpec's own comment), so
		// whatever would need one is denied instead of waiting forever for an answer nobody will
		// give. See defaultPermissionModes for the full reasoning.
		"--permission-prompts=none",
	}
	if mode != "" {
		args = append(args, "--permission-mode="+mode)
	}
	if spec.Model != "" {
		args = append(args, "--model="+c.mappedModel(spec.Model))
	}
	if spec.Thinking == "" {
		return args, nil
	}
	effort, ok := effortFor(spec.Thinking)
	if !ok {
		return nil, fmt.Errorf("%w: thinking mode %q", agents.ErrUnsupportedSetting, spec.Thinking)
	}
	return append(args, "--effort="+effort), nil
}

// startArgs is the full argument list for a new session: --session-id, the shared flags, and the
// role instructions, when there are any.
func startArgs(common []string, sessionID, instructions string) []string {
	extra := 1 // --session-id
	if instructions != "" {
		extra++ // --append-system-prompt
	}
	args := make([]string, 0, len(common)+extra)
	args = append(args, "--session-id="+sessionID)
	args = append(args, common...)
	if instructions != "" {
		args = append(args, "--append-system-prompt="+instructions)
	}
	return args
}

// resumeArgs is the full argument list to pick a session back up: --resume and the shared flags.
// Instructions are not resent; StartSpec.Instructions's own doc comment says they go with a new
// session's first message only.
func resumeArgs(common []string, sessionID string) []string {
	args := make([]string, 0, len(common)+1)
	args = append(args, "--resume="+sessionID)
	return append(args, common...)
}
