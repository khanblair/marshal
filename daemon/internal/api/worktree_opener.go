package api

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// What a person can ask the daemon to open a card's worktree with.
const (
	openWithFinder = "finder"
	openWithEditor = "editor"
)

// openTimeout cuts a program that was started to show a folder and did not return.
const openTimeout = 15 * time.Second

// Opener shows a card's worktree folder on the machine the daemon runs on. The folder always comes
// from the card's own record, never from a request, and with is openWithFinder or openWithEditor.
type Opener interface {
	Open(ctx context.Context, path, with string) error
}

// SystemOpener is the Opener of a real daemon. It starts the machine's own programs with fixed
// arguments and no shell: the file manager for a finder request, and the editor's command for an
// editor request, falling back to the file manager when the machine has none.
type SystemOpener struct {
	// Run starts a program and waits for it. Nil runs the real program; a test gives its own.
	Run func(ctx context.Context, name string, args ...string) error
	// LookPath says whether a program is installed. Nil asks the machine.
	LookPath func(name string) (string, error)
	// OS names the operating system. Empty means the one the daemon runs on.
	OS string
}

// Open runs the program for the request. A folder that cannot be shown is an error for the caller
// to turn into a sentence.
func (o SystemOpener) Open(ctx context.Context, path, with string) error {
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	switch with {
	case openWithFinder:
		return o.reveal(ctx, path)
	case openWithEditor:
		if _, err := o.lookPath("code"); err == nil {
			if err := o.run(ctx, "code", path); err == nil {
				return nil
			}
		}
		return o.show(ctx, path)
	}
	return fmt.Errorf("open a folder: %q is not a way to open it", with)
}

// reveal shows the folder in the file manager, selected in its parent where the machine can.
func (o SystemOpener) reveal(ctx context.Context, path string) error {
	if o.system() == "darwin" {
		return o.run(ctx, "open", "-R", path)
	}
	return o.run(ctx, "xdg-open", path)
}

// show opens the folder in the file manager.
func (o SystemOpener) show(ctx context.Context, path string) error {
	if o.system() == "darwin" {
		return o.run(ctx, "open", path)
	}
	return o.run(ctx, "xdg-open", path)
}

func (o SystemOpener) system() string {
	if o.OS != "" {
		return o.OS
	}
	return runtime.GOOS
}

func (o SystemOpener) run(ctx context.Context, name string, args ...string) error {
	if o.Run != nil {
		return o.Run(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...).Run()
}

func (o SystemOpener) lookPath(name string) (string, error) {
	if o.LookPath != nil {
		return o.LookPath(name)
	}
	return exec.LookPath(name)
}
