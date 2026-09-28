package codemap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The runner is where the map meets a program that may not be there, and may be the wrong program.
// These tests stand in for ctags and check both halves: which binary the runner will trust, and how
// it reads what one prints.

// fakeCommand is a program that answers canned output, remembering what it was run with.
type fakeCommand struct {
	stdout string
	err    error

	args  [][]string
	stdin []string
	dirs  []string
	bins  []string
}

func (f *fakeCommand) run(_ context.Context, dir, bin string, args []string, stdin string) ([]byte, error) {
	f.args = append(f.args, slices.Clone(args))
	f.stdin = append(f.stdin, stdin)
	f.dirs = append(f.dirs, dir)
	f.bins = append(f.bins, bin)
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.stdout), nil
}

// universalVersion is what a real universal ctags prints for `--version`.
const universalVersion = "Universal Ctags 6.2.0, Copyright (C) 2015 Universal Ctags Team\n"

// bsdUsage is what macOS's /usr/bin/ctags prints when it is asked for anything it does not take.
const bsdUsage = "ctags: illegal option -- -\nusage: ctags [-BFTaduwvx] [-f tagsfile] file ...\n"

// tagsSample is real universal ctags JSON output: a pseudo tag for ctags' own book-keeping, a
// function with a type, a method with a scope, and a type.
const tagsSample = `{"_type": "ptag", "name": "JSON_OUTPUT_VERSION", "path": "0.0", "pattern": "in development"}
{"_type": "tag", "name": "Send", "path": "internal/session/send.go", "pattern": "/^func (m *Manager) Send(/", "kind": "method", "scope": "Manager", "scopeKind": "type", "line": 27}
{"_type": "tag", "name": "Manager", "path": "internal/session/send.go", "pattern": "/^type Manager struct {/", "kind": "type", "line": 12}
{"_type": "tag", "name": "main", "path": "cmd/marshald/main.go", "pattern": "/^func main() {/", "kind": "function", "typeref": "typename:void", "line": 94}
`

func TestParseTagsReadsTheJSONOutput(t *testing.T) {
	matches, err := parseTags([]byte(tagsSample))
	if err != nil {
		t.Fatalf("read ctags' output: %v", err)
	}
	want := []Match{
		{
			Path: "internal/session/send.go", Line: 27, Kind: "method", Name: "Send",
			Parent: "Manager",
		},
		{Path: "internal/session/send.go", Line: 12, Kind: "type", Name: "Manager"},
		{Path: "cmd/marshald/main.go", Line: 94, Kind: "function", Name: "main"},
	}
	if !slices.Equal(matches, want) {
		t.Errorf("the parser answered\n%+v\nwant\n%+v", matches, want)
	}
}

// TestParseTagsLeavesOutCtagsOwnBookKeeping: the first line is ctags describing its output format,
// not a name in the project. Passing it on would put "JSON_OUTPUT_VERSION" in an agent's answer.
func TestParseTagsLeavesOutCtagsOwnBookKeeping(t *testing.T) {
	matches, err := parseTags([]byte(tagsSample))
	if err != nil {
		t.Fatalf("read ctags' output: %v", err)
	}
	for _, match := range matches {
		if strings.Contains(match.Name, "JSON_OUTPUT_VERSION") {
			t.Errorf("the parser passed on ctags' own line: %+v", match)
		}
	}
}

func TestParseTagsReadsTheLongKindNameWhenThereIsOne(t *testing.T) {
	matches, err := parseTags([]byte(`{"_type": "tag", "name": "Send", "path": "a.go", "kind": "f", "kindName": "function", "line": 3}`))
	if err != nil {
		t.Fatalf("read ctags' output: %v", err)
	}
	if len(matches) != 1 || matches[0].Kind != "function" {
		t.Errorf("the kind is %q, want the long name ctags gave", matches[0].Kind)
	}
}

// TestParseTagsRefusesALineItCannotRead: the output is written by a program, so a line the parser
// cannot read means the format is not the one it was written for. Answering as if that line had not
// been there would hide a broken map behind an ordinary-looking answer.
func TestParseTagsRefusesALineItCannotRead(t *testing.T) {
	_, err := parseTags([]byte("{\"_type\": \"tag\", \"name\": \"Send\"\n{}\n"))
	if err == nil {
		t.Fatal("a line the parser could not read was quietly dropped")
	}
	if !strings.Contains(err.Error(), "ctags") {
		t.Errorf("the failure is %q, which does not say whose output it was", err)
	}
}

func TestParseTagsAnswersNothingForNothing(t *testing.T) {
	matches, err := parseTags(nil)
	if err != nil {
		t.Fatalf("read empty output: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("empty output answered %+v", matches)
	}
}

func TestRunnerReadsSymbolsWithUniversalCtags(t *testing.T) {
	fake := &fakeCommand{stdout: universalVersion}
	runner := newCtagsRunner()
	runner.run = func(ctx context.Context, dir, bin string, args []string, stdin string) ([]byte, error) {
		// The version check is answered as a version; the tag run is answered as tags.
		if slices.Contains(args, "--version") {
			return fake.run(ctx, dir, bin, args, stdin)
		}
		return []byte(tagsSample), nil
	}

	matches, err := runner.Symbols(t.Context(), "/repo", []string{"internal/session/send.go"})
	if err != nil {
		t.Fatalf("read symbols: %v", err)
	}
	if len(matches) != 3 {
		t.Errorf("the runner answered %d symbols, want the 3 ctags printed", len(matches))
	}
	if notice := runner.Notice(); notice != "" {
		t.Errorf("the runner says %q although universal ctags is there", notice)
	}
}

// TestRunnerReadsTheFileNamesFromStandardInput: a repository has more files than an argument list
// can hold, so the names go in on stdin, one per line.
func TestRunnerReadsTheFileNamesFromStandardInput(t *testing.T) {
	var args []string
	var stdin string
	runner := newCtagsRunner()
	runner.run = func(_ context.Context, _ string, _ string, a []string, in string) ([]byte, error) {
		if slices.Contains(a, "--version") {
			return []byte(universalVersion), nil
		}
		args, stdin = a, in
		return nil, nil
	}

	if _, err := runner.Symbols(t.Context(), "/repo", []string{"a.go", "b.ts"}); err != nil {
		t.Fatalf("read symbols: %v", err)
	}
	for _, want := range []string{"--output-format=json", "-L", "-", "--sort=no"} {
		if !slices.Contains(args, want) {
			t.Errorf("ctags was run with %q, which is missing %s", args, want)
		}
	}
	if want := "a.go\nb.ts\n"; stdin != want {
		t.Errorf("ctags was given %q on its input, want %q", stdin, want)
	}
}

// TestRunnerTreatsBSDCtagsAsNoCTags: macOS ships a program with the same name that takes none of
// these arguments. Trusting the name would have the map fail on every search rather than fall back.
func TestRunnerTreatsBSDCtagsAsNoCTags(t *testing.T) {
	runner := newCtagsRunner()
	runner.run = func(_ context.Context, _ string, _ string, args []string, _ string) ([]byte, error) {
		if slices.Contains(args, "--version") {
			return []byte(bsdUsage), errors.New("exit status 1")
		}
		t.Errorf("ctags was run for tags although it is not universal ctags: %q", args)
		return nil, nil
	}

	matches, err := runner.Symbols(t.Context(), "/repo", []string{"a.go"})
	if err != nil {
		t.Fatalf("read symbols: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("a machine with no universal ctags answered %+v", matches)
	}
	if notice := runner.Notice(); notice != noticeMissing {
		t.Errorf("the notice is %q, want the one about a machine with no ctags", notice)
	}
}

// TestRunnerTreatsAnotherCTagsAsTheWrongOne: a `ctags` that answers `--version` without saying it is
// universal ctags is not one this map can use, and the notice says which of the two problems it is.
func TestRunnerTreatsAnotherCTagsAsTheWrongOne(t *testing.T) {
	runner := newCtagsRunner()
	runner.run = func(_ context.Context, _ string, _ string, _ []string, _ string) ([]byte, error) {
		return []byte("Exuberant Ctags 5.8\n"), nil
	}
	if got := runner.Notice(); got != noticeWrongBinary {
		t.Errorf("the notice is %q, want the one about the wrong ctags", got)
	}
}

func TestRunnerAsksAboutTheBinaryOnlyOnce(t *testing.T) {
	calls := 0
	runner := newCtagsRunner()
	runner.run = func(_ context.Context, _ string, _ string, args []string, _ string) ([]byte, error) {
		if slices.Contains(args, "--version") {
			calls++
			return []byte(universalVersion), nil
		}
		return nil, nil
	}
	for range 3 {
		runner.Notice()
		_, _ = runner.Symbols(t.Context(), "/repo", []string{"a.go"})
	}
	if calls != 1 {
		t.Errorf("the binary was asked its version %d times, want once per process", calls)
	}
}

func TestRunnerDoesNotRunCTagsForNoFiles(t *testing.T) {
	runner := newCtagsRunner()
	runner.run = func(_ context.Context, _ string, _ string, _ []string, _ string) ([]byte, error) {
		t.Error("ctags was run with no files to read")
		return nil, nil
	}
	matches, err := runner.Symbols(t.Context(), "/repo", nil)
	if err != nil || len(matches) != 0 {
		t.Errorf("reading no files answered %+v, %v", matches, err)
	}
}

func TestRunnerReportsACTagsThatFailed(t *testing.T) {
	runner := newCtagsRunner()
	runner.run = func(_ context.Context, _ string, _ string, args []string, _ string) ([]byte, error) {
		if slices.Contains(args, "--version") {
			return []byte(universalVersion), nil
		}
		return nil, errors.New("ctags --output-format=json: no such file")
	}
	if _, err := runner.Symbols(t.Context(), "/repo", []string{"gone.go"}); err == nil {
		t.Fatal("a ctags that failed answered as if the files had no symbols")
	}
}

// TestRunCommandAnswersWhatTheProgramPrinted runs a real program: the runner's own command is the
// one thing a fake cannot stand in for.
func TestRunCommandAnswersWhatTheProgramPrinted(t *testing.T) {
	out, err := runCommand(t.Context(), t.TempDir(), "/bin/echo", []string{"hello"}, "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Errorf("the program answered %q, want what it printed", out)
	}
}

// TestRunCommandReportsAProgramThatIsNotThere: the daemon must not take a missing binary for an
// empty answer.
func TestRunCommandReportsAProgramThatIsNotThere(t *testing.T) {
	_, err := runCommand(t.Context(), t.TempDir(), "marshal-no-such-program", nil, "")
	if err == nil {
		t.Fatal("a program that is not there was run as if it were")
	}
}

// TestUniversalCtagsAnswersTheRunnerOnThisMachine is the one check that only a real universal ctags
// can make: that the arguments the runner passes are the ones it takes. It is skipped where there is
// none, which is the ordinary case on a machine with macOS's BSD ctags and the case the fallback
// exists for.
func TestUniversalCtagsAnswersTheRunnerOnThisMachine(t *testing.T) {
	runner := NewRunner()
	if runner.Notice() != "" {
		t.Skipf("no universal ctags on this machine: %s", runner.Notice())
	}
	root := writeTree(t, "a.py")
	if err := os.WriteFile(filepath.Join(root, "a.py"), []byte("def send(message):\n    return message\n"), 0o644); err != nil {
		t.Fatalf("write the source file: %v", err)
	}
	matches, err := runner.Symbols(t.Context(), root, []string{"a.py"})
	if err != nil {
		t.Fatalf("read symbols from the real ctags: %v", err)
	}
	var names []string
	for _, match := range matches {
		names = append(names, match.Name)
	}
	if !slices.Contains(names, "send") {
		t.Errorf("the real ctags answered %q, want the function in the file", names)
	}
}
