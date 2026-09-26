package sample_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/sample"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// git runs a Git command in a folder that must succeed, and returns what it printed.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := testutil.Git().Run(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

// ensure makes the sample under a new data folder, or fails the test.
func ensure(t *testing.T, dataDir string) string {
	t.Helper()
	root, err := sample.Ensure(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("make the sample: %v", err)
	}
	return root
}

// The sample is a real repository on main, with its commits in order, by Marshal's own identity,
// with every file a person reads first, and nothing left to commit.
func TestTheSampleIsARealRepository(t *testing.T) {
	dataDir := t.TempDir()
	root := ensure(t, dataDir)
	if root != sample.Folder(dataDir) {
		t.Errorf("root = %s, want %s", root, sample.Folder(dataDir))
	}
	if got := git(t, root, "branch", "--show-current"); got != sample.Branch {
		t.Errorf("branch = %q, want %q", got, sample.Branch)
	}
	subjects := strings.Split(git(t, root, "log", "--format=%s"), "\n")
	want := []string{"Add due dates to todos", "Start the sample todo service"}
	if strings.Join(subjects, "|") != strings.Join(want, "|") {
		t.Errorf("commits = %q, want %q (newest first)", subjects, want)
	}
	if authors := git(t, root, "log", "--format=%an <%ae> %cn <%ce>"); strings.Count(authors, "Marshal <sample@marshal.invalid>") != 2*len(want) {
		t.Errorf("the commits are not all by the sample's identity:\n%s", authors)
	}
	if status := git(t, root, "status", "--porcelain"); status != "" {
		t.Errorf("the sample has changes left over:\n%s", status)
	}
	for _, name := range []string{".gitignore", "README.md", "package.json", "tsconfig.json", "src/server.ts", "src/todos.ts", "src/dates.ts"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Errorf("the sample has no %s: %v", name, err)
		}
	}
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || !strings.Contains(string(readme), "safe to try things on") {
		t.Errorf("the README does not say the sample is safe to try things on (%v)", err)
	}
}

// The sample does not use or need the person's Git identity or settings, and it does not write an
// identity into its own configuration, where it would take over the person's later commits.
func TestTheSampleDoesNotDependOnThePersonsGit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_AUTHOR_NAME", "Ada Lovelace")
	t.Setenv("GIT_AUTHOR_EMAIL", "ada@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Ada Lovelace")
	t.Setenv("GIT_COMMITTER_EMAIL", "ada@example.com")
	// A global setting that would make every commit fail if the sample read it.
	global := filepath.Join(home, "gitconfig")
	if err := os.WriteFile(global, []byte("[commit]\n\tgpgsign = true\n[gpg]\n\tprogram = /no/such/gpg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)

	root := ensure(t, t.TempDir())
	if authors := git(t, root, "log", "--format=%an <%ae>"); strings.Contains(authors, "Ada") {
		t.Errorf("the person's identity reached the sample's commits:\n%s", authors)
	}
	local, err := os.ReadFile(filepath.Join(root, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(local), "[user]") || strings.Contains(string(local), "email") {
		t.Errorf("the sample wrote an identity into its own configuration:\n%s", local)
	}
}

// Asking again uses the sample that is there, with whatever the person did in it, and changes
// nothing.
func TestAskingAgainUsesTheSampleThatIsThere(t *testing.T) {
	dataDir := t.TempDir()
	root := ensure(t, dataDir)
	head := git(t, root, "rev-parse", "HEAD")
	notes := filepath.Join(root, "notes.md")
	if err := os.WriteFile(notes, []byte("my own notes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if again := ensure(t, dataDir); again != root {
		t.Errorf("the second call gave %s, want %s", again, root)
	}
	if got := git(t, root, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD moved from %s to %s", head, got)
	}
	if _, err := os.Stat(notes); err != nil {
		t.Errorf("the person's own file is gone: %v", err)
	}
}

// With a fixed identity and fixed dates, the same files make the same commits on every machine.
func TestTheSampleIsTheSameEverywhere(t *testing.T) {
	a := git(t, ensure(t, t.TempDir()), "rev-parse", "HEAD")
	b := git(t, ensure(t, t.TempDir()), "rev-parse", "HEAD")
	if a != b {
		t.Errorf("two samples have different commits: %s and %s", a, b)
	}
}

// A folder in the sample's place that is not a repository is reported, and left as it is.
func TestAFolderThatIsNotARepositoryIsLeftAlone(t *testing.T) {
	dataDir := t.TempDir()
	folder := sample.Folder(dataDir)
	if err := os.MkdirAll(folder, 0o750); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(folder, "keep.txt")
	if err := os.WriteFile(kept, []byte("mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := sample.Ensure(context.Background(), dataDir)
	if !errors.Is(err, sample.ErrNotUsable) {
		t.Fatalf("err = %v, want ErrNotUsable", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("the file in the folder is gone: %v", err)
	}
}

// Two requests at once both get the one sample, and no staging folder is left behind.
func TestTwoRequestsAtOnceGetOneSample(t *testing.T) {
	dataDir := t.TempDir()
	const callers = 4
	roots := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() { roots[i], errs[i] = sample.Ensure(context.Background(), dataDir) })
	}
	wg.Wait()
	for i := range callers {
		if errs[i] != nil || roots[i] != sample.Folder(dataDir) {
			t.Errorf("call %d = %s, %v; want %s", i, roots[i], errs[i], sample.Folder(dataDir))
		}
	}
	entries, err := os.ReadDir(filepath.Dir(sample.Folder(dataDir)))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != sample.FolderName {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("the sample's parent folder holds %v, want only %s", names, sample.FolderName)
	}
}

// A request that is cancelled before it finishes leaves nothing in the sample's place, so the next
// one starts clean.
func TestACancelledRequestLeavesNoHalfMadeSample(t *testing.T) {
	dataDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sample.Ensure(ctx, dataDir); err == nil {
		t.Fatal("a cancelled request made the sample")
	}
	if _, err := os.Stat(sample.Folder(dataDir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a cancelled request left the sample folder: %v", err)
	}
	ensure(t, dataDir)
}

// Every commit that ships has a message and files inside the repository.
func TestEveryCommitHasAMessageAndFiles(t *testing.T) {
	commits, err := sample.Commits()
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) < 2 {
		t.Fatalf("the sample has %d commits, want a short history", len(commits))
	}
	for _, c := range commits {
		if c.Message == "" || len(c.Files) == 0 {
			t.Errorf("commit %+v has no message or no files", c)
		}
		for _, f := range c.Files {
			if !filepath.IsLocal(filepath.FromSlash(f.Name)) || f.Data == "" {
				t.Errorf("commit %q has a bad file %q", c.Message, f.Name)
			}
		}
	}
}
