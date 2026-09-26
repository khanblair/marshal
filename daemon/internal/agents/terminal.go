package agents

import "context"

// Terminal is what an agent that runs in a pseudo-terminal offers besides Agent: the calls that a
// terminal view needs. The PTY adapter (agents/pty) is the one implementation, and the session
// manager finds it by asking an Agent whether it is also a Terminal, so an agent that only speaks a
// structured protocol needs none of this.
type Terminal interface {
	// WriteRaw types bytes into the terminal exactly as they are, for a view that forwards what the
	// person types. It blocks while the program does not read its input, so a caller that must not
	// wait puts a queue in front of it.
	WriteRaw(ctx context.Context, h SessionHandle, data []byte) error
	// Resize sets the size of the terminal in character cells, and the program is told.
	Resize(ctx context.Context, h SessionHandle, cols, rows int) error
	// Size returns the size the terminal has now.
	Size(h SessionHandle) (cols, rows int, err error)
}
