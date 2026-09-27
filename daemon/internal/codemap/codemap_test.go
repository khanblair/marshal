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

// The map is what lets an agent ask where a name is instead of reading the tree to find it
// (docs/architecture.md section 3; build-plan task 7.9). These tests drive it over a temporary
// repository and a runner that stands in for ctags, so what is under test is the map's own
// behaviour: when it walks the tree, when it re-reads one file, and what it answers.

const testProject = "small-repo"

// fakeRunner is a ctags that answers canned symbols and remembers what it was asked to read.
type fakeRunner struct {
	// byPath is the symbols of each file, keyed by the path ctags would be given.
	byPath map[string][]Match
	// notice is what the runner says about itself: set it to stand in for a machine with no
	// universal ctags.
	notice string
	err    error

	// runs records the paths of every run, which is how a test says whether the map re-read the tree
	// or one file.
	runs  [][]string
	roots []string
}

func (f *fakeRunner) Symbols(_ context.Context, root string, paths []string) ([]Match, error) {
	f.runs = append(f.runs, slices.Clone(paths))
	f.roots = append(f.roots, root)
	if f.err != nil {
		return nil, f.err
	}
	var out []Match
	for _, path := range paths {
		for _, match := range f.byPath[path] {
			match.Path = path
			out = append(out, match)
		}
	}
	return out, nil
}

func (f *fakeRunner) Notice() string { return f.notice }

// repo writes a small repository into a temporary folder and answers its root, the runner, and a map
// over it.
func repo(t *testing.T, files map[string]string) (string, *fakeRunner, *Map) {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("make %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runner := &fakeRunner{byPath: map[string][]Match{}}
	m, err := New(Deps{
		Roots: func(_ context.Context, projectID string) (string, error) {
			if projectID != testProject {
				return "", errors.New("no such project")
			}
			return root, nil
		},
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("build the map: %v", err)
	}
	return root, runner, m
}

func TestSearchAnswersWhereANameIs(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"a.go": "package a\n", "b.ts": "export const b = 1\n"})
	runner.byPath["a.go"] = []Match{{Name: "Send", Kind: "function", Line: 27, Parent: "Manager"}}

	matches, err := m.Search(t.Context(), testProject, "Send", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	want := []Match{{Path: "a.go", Line: 27, Kind: "function", Name: "Send", Parent: "Manager"}}
	if !slices.Equal(matches, want) {
		t.Errorf("the map answered\n%+v\nwant\n%+v", matches, want)
	}
}

func TestSearchPutsTheNameItselfFirst(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"a.go": "package a\n"})
	runner.byPath["a.go"] = []Match{
		{Name: "Resend", Kind: "function", Line: 9},
		{Name: "SendMessage", Kind: "function", Line: 5},
		{Name: "Send", Kind: "method", Line: 3},
	}
	matches, err := m.Search(t.Context(), testProject, "send", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var names []string
	for _, match := range matches {
		names = append(names, match.Name)
	}
	if want := []string{"Send", "SendMessage", "Resend"}; !slices.Equal(names, want) {
		t.Errorf("the map answered %q, want the name itself, then the prefix, then the substring: %q",
			names, want)
	}
}

func TestSearchFindsAMethodByItsQualifiedName(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"a.go": "package a\n"})
	runner.byPath["a.go"] = []Match{{Name: "Send", Kind: "method", Line: 3, Parent: "Manager"}}

	matches, err := m.Search(t.Context(), testProject, "Manager.Send", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(matches) != 1 || matches[0].Parent != "Manager" {
		t.Errorf("the map answered %+v, want the method of Manager", matches)
	}
}

// TestSearchWalksTheTreeOnce: the index is built on the first search and kept. A second question is
// answered from what the daemon already read, which is what makes the map worth having in a process
// that runs for days.
func TestSearchWalksTheTreeOnce(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"a.go": "package a\n", "b.go": "package b\n"})
	runner.byPath["a.go"] = []Match{{Name: "First", Kind: "function", Line: 1}}

	if _, err := m.Search(t.Context(), testProject, "First", 0); err != nil {
		t.Fatalf("first search: %v", err)
	}
	if _, err := m.Search(t.Context(), testProject, "Second", 0); err != nil {
		t.Fatalf("second search: %v", err)
	}
	if len(runner.runs) != 1 {
		t.Fatalf("the tree was read %d times, want once", len(runner.runs))
	}
	if want := []string{"a.go", "b.go"}; !slices.Equal(runner.runs[0], want) {
		t.Errorf("the first run read %q, want every source file in name order: %q", runner.runs[0], want)
	}
}

// TestUpdateReReadsOnlyTheFileThatChanged: this is the whole point of the incremental seam. After the
// first build, a write is one ctags run over one path, never another walk.
func TestUpdateReReadsOnlyTheFileThatChanged(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"a.go": "package a\n", "b.go": "package b\n"})
	runner.byPath["a.go"] = []Match{{Name: "Old", Kind: "function", Line: 1}}
	runner.byPath["b.go"] = []Match{{Name: "Kept", Kind: "function", Line: 1}}
	if _, err := m.Search(t.Context(), testProject, "Kept", 0); err != nil {
		t.Fatalf("first search: %v", err)
	}

	// a.go is edited: Old is gone and New is there.
	runner.byPath["a.go"] = []Match{{Name: "New", Kind: "function", Line: 4}}
	if err := m.Update(t.Context(), testProject, "a.go"); err != nil {
		t.Fatalf("update: %v", err)
	}

	if len(runner.runs) != 2 {
		t.Fatalf("ctags ran %d times, want a build and one update", len(runner.runs))
	}
	if want := []string{"a.go"}; !slices.Equal(runner.runs[1], want) {
		t.Errorf("the update read %q, want only the file that changed: %q", runner.runs[1], want)
	}

	matches, err := m.Search(t.Context(), testProject, "New", 0)
	if err != nil {
		t.Fatalf("search after the update: %v", err)
	}
	if len(matches) != 1 || matches[0].Line != 4 {
		t.Errorf("the map answered %+v, want the edited file's new symbol", matches)
	}
	if gone, err := m.Search(t.Context(), testProject, "Old", 0); err != nil {
		t.Fatalf("search for what left: %v", err)
	} else if len(gone) != 0 {
		t.Errorf("the map still answers %+v for a name that was removed", gone)
	}
	if kept, err := m.Search(t.Context(), testProject, "Kept", 0); err != nil {
		t.Fatalf("search for the other file: %v", err)
	} else if len(kept) != 1 {
		t.Errorf("the update lost the other file's symbols: %+v", kept)
	}

	// The runner was told the project's root both times: an update reads the file where it lives.
	if runner.roots[1] != runner.roots[0] {
		t.Errorf("the update ran in %q rather than the project root %q", runner.roots[1], runner.roots[0])
	}
}

// TestUpdateBeforeTheFirstSearchDoesNothing: a project the daemon has never indexed has nothing to
// keep true, and the next search builds the whole thing anyway.
func TestUpdateBeforeTheFirstSearchDoesNothing(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"a.go": "package a\n"})
	if err := m.Update(t.Context(), testProject, "a.go"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(runner.runs) != 0 {
		t.Errorf("ctags ran %d times before anything was indexed", len(runner.runs))
	}
}

func TestRemoveForgetsAFile(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"a.go": "package a\n", "b.go": "package b\n"})
	runner.byPath["a.go"] = []Match{{Name: "Gone", Kind: "function", Line: 1}}
	if _, err := m.Search(t.Context(), testProject, "Gone", 0); err != nil {
		t.Fatalf("first search: %v", err)
	}

	m.Remove(testProject, "a.go")
	if matches, err := m.Search(t.Context(), testProject, "Gone", 0); err != nil {
		t.Fatalf("search after the removal: %v", err)
	} else if len(matches) != 0 {
		t.Errorf("the map answers %+v for a file that went", matches)
	}
	// And the file is not offered by name either.
	if matches, err := m.Search(t.Context(), testProject, "a.go", 0); err != nil {
		t.Fatalf("search for the file's name: %v", err)
	} else if len(matches) != 0 {
		t.Errorf("the map offers %+v for a file that went", matches)
	}
}

// TestSearchFallsBackToFileNames: a machine with no universal ctags still gets an answer, and the
// answer says it is answering from names alone.
func TestSearchFallsBackToFileNames(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"src/board.ts": "export const board = 1\n"})
	runner.notice = noticeMissing

	matches, err := m.Search(t.Context(), testProject, "board", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	want := []Match{{Path: "src/board.ts", Kind: fileKind, Name: "board.ts"}}
	if !slices.Equal(matches, want) {
		t.Errorf("the map answered\n%+v\nwant the file it could still see\n%+v", matches, want)
	}
	if m.Notice() != noticeMissing {
		t.Errorf("the map's notice is %q, want the runner's %q", m.Notice(), noticeMissing)
	}
}

// TestSearchPrefersSymbolsToFileNames: a file-name match is the fallback, not a second helping. An
// agent that asked for a name gets the name it asked for.
func TestSearchPrefersSymbolsToFileNames(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"send.ts": "export const send = 1\n"})
	runner.byPath["send.ts"] = []Match{{Name: "Send", Kind: "function", Line: 2}}

	matches, err := m.Search(t.Context(), testProject, "send", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(matches) != 1 || matches[0].Kind != "function" {
		t.Errorf("the map answered %+v, want the symbol and not the file it is in", matches)
	}
}

func TestSearchCapsTheLimit(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"a.go": "package a\n"})
	for i := range 10 {
		runner.byPath["a.go"] = append(runner.byPath["a.go"],
			Match{Name: "Handle", Kind: "function", Line: i + 1})
	}
	matches, err := m.Search(t.Context(), testProject, "Handle", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(matches) != 3 {
		t.Errorf("the map answered %d matches, want the 3 it was asked for", len(matches))
	}
	// A caller that asks for more than the map will give gets the most it gives.
	all, err := m.Search(t.Context(), testProject, "Handle", maxLimit+1000)
	if err != nil {
		t.Fatalf("search with a huge limit: %v", err)
	}
	if len(all) != 10 {
		t.Errorf("the map answered %d matches, want all 10 it has", len(all))
	}
}

func TestForgetDropsTheWholeIndex(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"a.go": "package a\n"})
	runner.byPath["a.go"] = []Match{{Name: "Send", Kind: "function", Line: 1}}
	if _, err := m.Search(t.Context(), testProject, "Send", 0); err != nil {
		t.Fatalf("first search: %v", err)
	}
	m.Forget(testProject)
	if _, err := m.Search(t.Context(), testProject, "Send", 0); err != nil {
		t.Fatalf("search after forgetting: %v", err)
	}
	if len(runner.runs) != 2 {
		t.Errorf("ctags ran %d times, want the tree to be read again after the index was dropped",
			len(runner.runs))
	}
}

func TestSearchWithNoQueryAnswersNothing(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"a.go": "package a\n"})
	matches, err := m.Search(t.Context(), testProject, "   ", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if matches != nil {
		t.Errorf("a blank query answered %+v", matches)
	}
	if len(runner.runs) != 0 {
		t.Errorf("a blank query read the tree %d times", len(runner.runs))
	}
}

func TestSearchReportsAProjectItCannotFind(t *testing.T) {
	_, _, m := repo(t, map[string]string{"a.go": "package a\n"})
	_, err := m.Search(t.Context(), "no-such-project", "Send", 0)
	if err == nil {
		t.Fatal("a project that cannot be found answered as if it had no matches")
	}
	if !strings.Contains(err.Error(), "no-such-project") {
		t.Errorf("the failure is %q, which does not name the project", err)
	}
}

func TestSearchReportsARunnerThatFails(t *testing.T) {
	_, runner, m := repo(t, map[string]string{"a.go": "package a\n"})
	runner.err = errors.New("ctags: illegal option -- -")
	_, err := m.Search(t.Context(), testProject, "Send", 0)
	if err == nil {
		t.Fatal("a runner that failed answered as if the project had no symbols")
	}
	if !strings.Contains(err.Error(), "illegal option") {
		t.Errorf("the failure is %q, which loses what ctags said", err)
	}
}

func TestNewRequiresAWayToFindAProjectsFolder(t *testing.T) {
	if _, err := New(Deps{}); err == nil {
		t.Fatal("a map with no way to find a project's folder was built")
	}
}

// TestNewFindsTheProjectsFolderOnlyWhenItIndexes: a project that is never searched is never walked,
// so a daemon with fifty projects does not read fifty trees at start-up.
func TestNewFindsTheProjectsFolderOnlyWhenItIndexes(t *testing.T) {
	var asked []string
	m, err := New(Deps{
		Roots: func(_ context.Context, projectID string) (string, error) {
			asked = append(asked, projectID)
			return "", errors.New("nowhere")
		},
		Runner: &fakeRunner{byPath: map[string][]Match{}},
	})
	if err != nil {
		t.Fatalf("build the map: %v", err)
	}
	if len(asked) != 0 {
		t.Fatalf("the map looked for %q before it was asked anything", asked)
	}
	if _, err := m.Search(t.Context(), testProject, "Send", 0); err == nil {
		t.Fatal("a project whose folder could not be found answered as if it had no matches")
	}
	if want := []string{testProject}; !slices.Equal(asked, want) {
		t.Errorf("the map looked for %q, want %q", asked, want)
	}
}
