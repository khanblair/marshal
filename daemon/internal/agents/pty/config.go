package pty

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"
)

const (
	// DefaultCols is the width of a new terminal when the config says nothing.
	DefaultCols = 120
	// DefaultRows is the height of a new terminal when the config says nothing.
	DefaultRows = 32
	// MaxSize is the largest number of columns or rows. It is what fits the pseudo console on
	// every platform, and far more than any screen.
	MaxSize = 32767

	// RingBytes is how much of the latest output a session keeps for a viewer that opens late.
	RingBytes = 256 << 10
	// MaxEventBytes is the most that one TerminalOutput event carries.
	MaxEventBytes = 16 << 10
	// FlushInterval is how long output waits to be joined with more of it before it goes out as
	// an event. It bounds the delay a viewer sees, and the event rate of a chatty program. A
	// full MaxEventBytes goes out at once, so a program that prints a lot is not slowed down.
	FlushInterval = 30 * time.Millisecond

	defaultStopGrace = 3 * time.Second
	// terminalName is the TERM value that programs see. Modern CLIs draw colors and boxes for it.
	terminalName = "xterm-256color"
	// interruptByte is what a terminal sends for Ctrl-C.
	interruptByte = 0x03
)

// Config says how to run one CLI in a terminal. One Adapter runs many sessions of it.
type Config struct {
	// Path is the program. It is required.
	Path string
	// Args are the arguments of a new session.
	Args []string
	// Env is added to the environment of every session, as KEY=value entries. A session's own
	// StartSpec.Env comes after it. Beyond these, a session sees only what internal/proc lets
	// through from the daemon's environment, plus TERM.
	Env []string
	// Cols and Rows are the size of the terminal. Zero means DefaultCols and DefaultRows.
	Cols, Rows int
	// StartArgs, when set, gives the arguments of a new session in place of Args, and is told the
	// session id that the adapter made up. Use it for a CLI that lets the caller choose the
	// session id, so that ResumeArgs can name it later.
	StartArgs func(sessionID string) []string
	// ResumeArgs gives the arguments that pick up a session that an earlier process had. A nil
	// value means the CLI cannot resume, and Resume returns agents.ErrCannotResume.
	ResumeArgs func(sessionID string) []string
	// InstructionArgs turns the role instructions of a new session into extra arguments, for
	// example a system prompt flag, added after the others. A nil value means the CLI has no
	// such flag, and the instructions are not delivered: there is no reliable way to type them
	// into a terminal before the CLI is ready. Resume never sends them again.
	InstructionArgs func(instructions string) []string
	// LineEnd is what Send types after the text. The default is a newline on Unix and a carriage
	// return on Windows. A CLI that draws its own screen in raw mode wants "\r", the key that
	// Enter sends.
	LineEnd string
	// StopGrace is how long Stop waits for the program to exit on its own before it kills the
	// whole process tree. The default is 3 seconds.
	StopGrace time.Duration
	// Tee, when set, receives every byte of output of every session, as it is read, before it is
	// batched. It is for the session log. Writes are one at a time, and a slow Tee slows the
	// program down. An adapter that serves one session at a time gives that session a log of
	// its own. A Tee that fails is dropped after one warning.
	Tee io.Writer
	// Logger receives the adapter's log lines. The default is slog.Default().
	Logger *slog.Logger
}

// withDefaults checks the config and fills in what was left out.
func (c Config) withDefaults() (Config, error) {
	if c.Path == "" {
		return Config{}, errors.New("pty adapter: the path of the program is required")
	}
	if c.Cols < 0 || c.Rows < 0 || c.Cols > MaxSize || c.Rows > MaxSize {
		return Config{}, fmt.Errorf("pty adapter: the terminal size %dx%d is out of range", c.Cols, c.Rows)
	}
	if c.Cols == 0 {
		c.Cols = DefaultCols
	}
	if c.Rows == 0 {
		c.Rows = DefaultRows
	}
	if c.LineEnd == "" {
		c.LineEnd = defaultLineEnd
	}
	if c.StopGrace <= 0 {
		c.StopGrace = defaultStopGrace
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c, nil
}
