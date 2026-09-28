package codemap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// This file is the runner the daemon uses: universal ctags, run as an external program.
//
// # Which ctags
//
// Universal ctags answers `--output-format=json`, which is what makes its output something to read
// rather than something to parse out of a tags file. macOS ships a different program with the same
// name - BSD ctags, in /usr/bin - that takes none of these arguments and fails with "illegal option"
// if it is given them. The name on the PATH therefore proves nothing, so the runner asks the binary
// for its version once and only trusts it if it says it is Universal Ctags. Anything else is treated
// as no ctags at all: the map answers from file names and Notice says why.

const (
	// defaultBinary is the program the runner runs. It is the name universal ctags installs itself
	// as.
	defaultBinary = "ctags"
	// universalMarker is what `ctags --version` prints when the binary really is universal ctags.
	universalMarker = "Universal Ctags"
	// probeTimeout bounds the version check, so a binary that hangs on `--version` cannot hold a
	// search.
	probeTimeout = 5 * time.Second
	// killWaitDelay is how long a cancelled ctags is given to let go of its output, so a cancelled
	// index cannot hang on a pipe a child still holds.
	killWaitDelay = 5 * time.Second
	// symbolsTimeout bounds one ctags run that reads a whole tree. An index build is the only full
	// walk the map does, and it should not be able to run forever behind an agent's call.
	symbolsTimeout = 2 * time.Minute
)

// The two notices. They are sentences for an agent, so they say what is missing, what the answer
// is instead, and what to install - the three things a model needs to decide whether reading the
// file it was told about is worth it.
const (
	noticeMissing = "Marshal cannot read symbols on this machine, because universal ctags is not " +
		"installed, so this search answered from file names alone. Install universal ctags for " +
		"matches by symbol."
	noticeWrongBinary = "Marshal cannot read symbols on this machine, because the `ctags` command " +
		"is BSD ctags rather than universal ctags, so this search answered from file names alone. " +
		"Install universal ctags and put it ahead of /usr/bin/ctags on the PATH for matches by " +
		"symbol."
)

// command runs a program and answers its standard output. It is a field on the runner rather than a
// bare call to the os/exec package so a test can stand in for the program.
type command func(ctx context.Context, dir, bin string, args []string, stdin string) ([]byte, error)

// ctagsRunner reads symbols with universal ctags.
type ctagsRunner struct {
	bin string
	run command

	// noticeOnce guards the version check: the binary on the PATH does not change under a running
	// daemon, so it is asked once.
	noticeOnce sync.Once
	present    bool
	notice     string
}

// NewRunner answers the runner the daemon uses: universal ctags read as a program.
func NewRunner() Runner { return newCtagsRunner() }

func newCtagsRunner() *ctagsRunner {
	return &ctagsRunner{bin: defaultBinary, run: runCommand}
}

// Symbols runs ctags over the named files and answers what they declare. It answers no symbols and
// no error when there is no universal ctags to run: the caller finds out why from Notice, and the
// map falls back to file names.
func (r *ctagsRunner) Symbols(ctx context.Context, root string, paths []string) ([]Match, error) {
	if len(paths) == 0 || !r.available(ctx) {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, symbolsTimeout)
	defer cancel()

	// ctags reads the file names from its standard input rather than its arguments: a repository has
	// more files than an argument list can hold.
	out, err := r.run(ctx, root, r.bin,
		[]string{"--output-format=json", "--fields=+nK", "--sort=no", "-L", "-"},
		strings.Join(paths, "\n")+"\n")
	if err != nil {
		return nil, err
	}
	return parseTags(out)
}

// Notice is the sentence to give an agent when there is no universal ctags to read symbols with.
func (r *ctagsRunner) Notice() string {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	r.available(ctx)
	return r.notice
}

// available reports whether a universal ctags could be run, asking once.
func (r *ctagsRunner) available(ctx context.Context) bool {
	r.noticeOnce.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		out, err := r.run(ctx, "", r.bin, []string{"--version"}, "")
		switch {
		case err != nil:
			r.notice = noticeMissing
		case !strings.Contains(string(out), universalMarker):
			r.notice = noticeWrongBinary
		default:
			r.present = true
		}
	})
	return r.present
}

// tagLine is one line of universal ctags' JSON output. Only the fields the map uses are read; ctags
// writes more of them.
type tagLine struct {
	// Type is "_type": "tag" for a name in a file, and "ptag" for ctags' own book-keeping, which is
	// not a name in the project and is skipped.
	Type string `json:"_type"`
	// Name is the symbol's name.
	Name string `json:"name"`
	// Path is the file it is in.
	Path string `json:"path"`
	// Kind is what it is, as a long name ("function", "method") when ctags gives one.
	Kind string `json:"kind"`
	// KindName is the same thing under the name ctags uses for some languages.
	KindName string `json:"kindName"`
	// Scope is the type or namespace the name is in, when ctags knows one.
	Scope string `json:"scope"`
	// Line is the one-based line the name is on.
	Line int `json:"line"`
}

// parseTags reads universal ctags' JSON output, one object per line.
//
// A line that cannot be read is an error rather than a line quietly dropped: the output is written
// by a program, so a line the map cannot read means the format is not the one this parser was
// written for, and answering as if that line had not been there would hide a broken map.
func parseTags(out []byte) ([]Match, error) {
	decoder := json.NewDecoder(bytes.NewReader(out))
	var matches []Match
	for {
		var tag tagLine
		err := decoder.Decode(&tag)
		if errors.Is(err, io.EOF) {
			return matches, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read ctags' output: %w", err)
		}
		if tag.Type == "ptag" || tag.Name == "" {
			continue
		}
		kind := tag.KindName
		if kind == "" {
			kind = tag.Kind
		}
		matches = append(matches, Match{
			Path: tag.Path, Line: tag.Line, Kind: kind, Name: tag.Name, Parent: tag.Scope,
		})
	}
}

// runCommand runs the program and answers its standard output, with the file names on its standard
// input.
func runCommand(ctx context.Context, dir, bin string, args []string, stdin string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	cmd.WaitDelay = killWaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errors.New("universal ctags is not installed, or is not on the PATH")
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("ctags %s: %s", strings.Join(args, " "), detail)
	}
	return stdout.Bytes(), nil
}
