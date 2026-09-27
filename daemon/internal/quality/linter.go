package quality

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/proc"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The first layer of the checks: a project's own linters (docs/marshal-product-scope.md 15.4,
// docs/architecture.md section 17.2). Marshal runs each linter a project named in its smell profile
// in the card's worktree, on the files the card changed, with the project's own configuration, and
// files what it finds under the family the profile gives it.
//
// A linter is a program, so it sits behind the Linter interface: the service never depends on which
// linter it was given, and a test drives the whole pipeline with a fake and never starts a process.
// CommandLinter is the real one, and it is the only thing here that starts a program: it goes
// through internal/proc, which starts a child with a list of arguments and never a shell, in its own
// process group, with a filtered environment, and always reaps it.
//
// A linter is run only with a time limit and only while a check is asked for. Checks never run during
// daemon idle, and a linter that writes more than MaxLintBytes has the rest of its output dropped
// rather than read forever.

const (
	// DefaultLintTimeout bounds one linter's run. A linter that has not answered by then is killed,
	// because a check that hangs is worse than a check that was skipped.
	DefaultLintTimeout = 2 * time.Minute
	// DefaultLintBytes is how much of one linter's output is read before the rest is dropped.
	DefaultLintBytes = 1 << 20
)

// LintRequest is what a linter is given: the worktree to run in, the files the card changed, and the
// language they are in when they share one.
type LintRequest struct {
	// Dir is the card's worktree, which the linter runs in.
	Dir string
	// Files are the paths the card changed, relative to the worktree, with forward slashes. They are
	// appended to the linter's command as its last arguments.
	Files []string
	// Language is the language of the changed files when they share one, so a linter that only fits
	// one language can be skipped for another. Empty when the files are of several languages.
	Language string
}

// LinterFinding is one thing a linter reported.
type LinterFinding struct {
	// File is the path the linter named, relative to the worktree.
	File string
	// Line is the line in that file. Zero when the linter named only the file.
	Line int
	// Message is what the linter said, in its own words.
	Message string
	// Family is the smell family the linter's findings belong to, from the profile.
	Family protocol.SmellFamily
}

// Linter runs one of a project's own linters over a card's changed files. A linter that finds
// nothing answers an empty list, and one that could not be run answers an error, which the service
// turns into a finding's worth of nothing and a log line rather than a failed check.
type Linter interface {
	Lint(ctx context.Context, req LintRequest) ([]LinterFinding, error)
}

// CommandLinter is a project's linter as a program to run. Its command is a program and its
// arguments, and the changed files are appended, which is how nearly every linter takes them.
type CommandLinter struct {
	name    string
	command []string
	family  protocol.SmellFamily
	timeout time.Duration
	maxRead int64
}

// NewCommandLinter builds a linter from a profile entry. A command with no program is refused here
// as well as when the profile is saved, so a linter can never be built that would run nothing.
func NewCommandLinter(linter protocol.SmellLinter) (*CommandLinter, error) {
	if len(linter.Command) == 0 || strings.TrimSpace(linter.Command[0]) == "" {
		return nil, errors.New("quality: a linter needs a program to run")
	}
	if strings.TrimSpace(linter.Name) == "" {
		return nil, errors.New("quality: a linter needs a name")
	}
	family := linter.Family
	if family == "" {
		family = protocol.SmellFamilyLexicalAbusers
	}
	return &CommandLinter{
		name: strings.TrimSpace(linter.Name), command: append([]string{}, linter.Command...),
		family: family, timeout: DefaultLintTimeout, maxRead: DefaultLintBytes,
	}, nil
}

// Name is the linter's label, which its findings carry.
func (c *CommandLinter) Name() string { return c.name }

// Lint runs the linter in the worktree with the changed files appended, and reads the
// `file:line: message` lines it printed. A linter that exits non-zero is not an error: finding
// something is how a linter says it found something.
func (c *CommandLinter) Lint(ctx context.Context, req LintRequest) ([]LinterFinding, error) {
	if strings.TrimSpace(req.Dir) == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	args := append(append([]string{}, c.command[1:]...), req.Files...)
	child, err := proc.Start(ctx, proc.Spec{Path: c.command[0], Args: args, Dir: req.Dir})
	if err != nil {
		return nil, fmt.Errorf("run the linter %s: %w", c.name, err)
	}
	output, readErr := readAtMost(child.Stdout, c.maxRead)
	// Stop ends the whole tree, which is what makes a linter that spawned a helper quit too. It is
	// safe after a clean exit, and it is what bounds a linter that keeps printing past the read.
	_ = child.Stop(context.WithoutCancel(ctx), lintStopGrace)
	exit := child.Wait()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, fmt.Errorf("read what the linter %s printed: %w", c.name, readErr)
	}
	if exit.Err != nil && output == "" {
		return nil, fmt.Errorf("run the linter %s: %w", c.name, exit.Err)
	}
	return parseLinterOutput(output, c.family), nil
}

// lintStopGrace is how long a linter is given to stop politely before it is killed.
const lintStopGrace = 2 * time.Second

// readAtMost reads a stream up to limit bytes, so a linter that prints without end cannot fill the
// daemon's memory.
func readAtMost(r io.Reader, limit int64) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit))
	return string(data), err
}

// parseLinterOutput reads the `file:line: message` lines linters print, with or without a column.
// Every other line is ignored, so a linter's own banner or summary is not mistaken for a finding.
func parseLinterOutput(output string, family protocol.SmellFamily) []LinterFinding {
	var found []LinterFinding
	for _, line := range strings.Split(output, "\n") {
		file, number, message, ok := splitLinterLine(strings.TrimSpace(line))
		if !ok {
			continue
		}
		found = append(found, LinterFinding{File: file, Line: number, Message: message, Family: family})
	}
	return found
}

// splitLinterLine splits one `path:line[:col]: message` line.
func splitLinterLine(line string) (file string, number int, message string, ok bool) {
	if line == "" {
		return "", 0, "", false
	}
	first := strings.Index(line, ":")
	if first <= 0 {
		return "", 0, "", false
	}
	file = strings.TrimSpace(line[:first])
	rest := line[first+1:]
	numberText, rest, hasColon := cut(rest, ":")
	if !hasColon {
		return "", 0, "", false
	}
	value, err := strconv.Atoi(strings.TrimSpace(numberText))
	if err != nil || value <= 0 {
		return "", 0, "", false
	}
	// An optional column is dropped: Marshal reports the line, and the column is the linter's own.
	if next, after, ok := cut(rest, ":"); ok {
		if _, err := strconv.Atoi(strings.TrimSpace(next)); err == nil {
			rest = after
		}
	}
	message = strings.TrimSpace(rest)
	if message == "" {
		return "", 0, "", false
	}
	return file, value, message, true
}

// cut splits text at the first sep and reports whether sep was there.
func cut(text, sep string) (before, after string, found bool) {
	index := strings.Index(text, sep)
	if index < 0 {
		return text, "", false
	}
	return text[:index], text[index+len(sep):], true
}

// buildLinters turns a profile's linters into the programs to run. A linter that cannot be built is
// skipped with a warning rather than failing the whole check: one bad entry must not stop the others.
func buildLinters(profile protocol.SmellProfile, warn func(linter string, err error)) []Linter {
	var out []Linter
	for _, entry := range profile.Linters {
		linter, err := NewCommandLinter(entry)
		if err != nil {
			if warn != nil {
				warn(entry.Name, err)
			}
			continue
		}
		out = append(out, linter)
	}
	return out
}
