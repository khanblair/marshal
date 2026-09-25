package gitx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

func TestInspectFixtures(t *testing.T) {
	for _, name := range []string{"small-repo", "monorepo"} {
		t.Run(name, func(t *testing.T) {
			dir := fixture(t, name)
			info, err := testGit().Inspect(context.Background(), dir)
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if !sameFolder(t, info.Root, dir) {
				t.Errorf("Root = %s, want the folder %s", info.Root, dir)
			}
			if info.CurrentBranch != "main" || info.DefaultBranch != "main" {
				t.Errorf("branches = %q and %q, want main and main", info.CurrentBranch, info.DefaultBranch)
			}
			if !info.HasCommits || !info.Clean || len(info.Remotes) != 0 {
				t.Errorf("HasCommits, Clean, Remotes = %v, %v, %v; want true, true, none",
					info.HasCommits, info.Clean, info.Remotes)
			}
		})
	}
}

func TestInspectRootIsWhereTheFileSystemPutsIt(t *testing.T) {
	// On macOS the temp folder is /var/..., a link to /private/var/..., and Git reports the real one.
	dir := fixture(t, "small-repo")
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := testGit().Inspect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !sameFolder(t, info.Root, real) {
		t.Errorf("Root = %s, want %s", info.Root, real)
	}
	// Windows can spell the same folder with short names, so only the other systems are held to
	// the exact spelling.
	if runtime.GOOS != "windows" && info.Root != real {
		t.Errorf("Root = %s, want the spelling with links followed, %s", info.Root, real)
	}
}

func TestInspectAcceptsOtherSpellingsOfTheRoot(t *testing.T) {
	dir := fixture(t, "small-repo")
	links := filepath.Join(t.TempDir(), "link")
	symlink(t, dir, links)
	spaced := filepath.Join(t.TempDir(), "my project (v2)")
	if err := os.Mkdir(spaced, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, spaced, "init", "--quiet", "--initial-branch=main")
	tests := map[string]string{
		"trailing separator": dir + string(os.PathSeparator),
		"a link":             links,
		"dot segments":       dir + string(os.PathSeparator) + "src" + string(os.PathSeparator) + "..",
		"a space and parens": spaced,
	}
	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := testGit().Inspect(context.Background(), path); err != nil {
				t.Errorf("Inspect(%q): %v", path, err)
			}
		})
	}
}

func TestInspectIgnoresCaseWhereTheFileSystemDoes(t *testing.T) {
	dir := fixture(t, "small-repo")
	swapped := filepath.Join(filepath.Dir(dir), strings.ToUpper(filepath.Base(dir)))
	if _, err := os.Stat(swapped); err != nil {
		t.Skip("this file system tells upper and lower case apart")
	}
	if _, err := testGit().Inspect(context.Background(), swapped); err != nil {
		t.Errorf("Inspect(%q): %v", swapped, err)
	}
}

func TestInspectRefusesWhatIsNotTheTopOfAWorkingTree(t *testing.T) {
	dir := fixture(t, "small-repo")
	bare := filepath.Join(t.TempDir(), "bare.git")
	git(t, dir, "clone", "--quiet", "--bare", dir, bare)
	tests := map[string]string{
		"a subfolder":     filepath.Join(dir, "src"),
		"the .git folder": filepath.Join(dir, ".git"),
		"a bare repo":     bare,
		"a plain folder":  t.TempDir(),
		"a missing path":  filepath.Join(t.TempDir(), "nowhere"),
		"a file":          filepath.Join(dir, "README.md"),
	}
	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := testGit().Inspect(context.Background(), path)
			if !errors.Is(err, gitx.ErrNotARepo) {
				t.Fatalf("Inspect(%q) = %v, want ErrNotARepo", path, err)
			}
			assertPlainMessage(t, err)
		})
	}
}

// assertPlainMessage checks that an error reads as a plain sentence, with nothing from Git in it.
func assertPlainMessage(t *testing.T, err error) {
	t.Helper()
	msg := err.Error()
	if msg == "" || msg != strings.ToLower(msg[:1])+msg[1:] && !strings.HasPrefix(msg, "Git ") {
		t.Errorf("error %q does not start with a lower case letter", msg)
	}
	for _, jargon := range []string{"fatal", "exit status", "rev-parse", "git -c"} {
		if strings.Contains(msg, jargon) {
			t.Errorf("error %q has %q in it", msg, jargon)
		}
	}
}

func TestInspectTellsTheTopFolderForASubfolder(t *testing.T) {
	dir := fixture(t, "small-repo")
	_, err := testGit().Inspect(context.Background(), filepath.Join(dir, "src"))
	real, _ := filepath.EvalSymlinks(dir)
	if err == nil || !strings.Contains(err.Error(), "top folder") || !strings.Contains(err.Error(), filepath.Base(real)) {
		t.Errorf("error = %v, want one that names the top folder", err)
	}
}

func TestInspectReportsChanges(t *testing.T) {
	tests := map[string]func(t *testing.T, dir string){
		"an untracked file": func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "new.txt"), "new")
		},
		"an edited file": func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "README.md"), "edited")
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			dir := fixture(t, "small-repo")
			change(t, dir)
			info, err := testGit().Inspect(context.Background(), dir)
			if err != nil || info.Clean {
				t.Errorf("Clean = %v, %v; want false", info.Clean, err)
			}
		})
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInspectRemovesCredentialsFromRemotes(t *testing.T) {
	dir := fixture(t, "small-repo")
	git(t, dir, "remote", "add", "origin", "https://someone:s3cr3t@example.com/acme/web.git")
	git(t, dir, "remote", "add", "up.stream", "git@github.com:acme/web.git")
	git(t, dir, "remote", "add", "token", "https://ghp_abc123@example.com/acme/web.git")
	git(t, dir, "remote", "add", "shell", "ssh://git:pw@example.com:2222/acme/web.git")
	info, err := testGit().Inspect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []gitx.Remote{
		{Name: "origin", URL: "https://example.com/acme/web.git"},
		{Name: "shell", URL: "ssh://git@example.com:2222/acme/web.git"},
		{Name: "token", URL: "https://example.com/acme/web.git"},
		{Name: "up.stream", URL: "git@github.com:acme/web.git"},
	}
	if len(info.Remotes) != len(want) {
		t.Fatalf("Remotes = %v, want %v", info.Remotes, want)
	}
	for i := range want {
		if info.Remotes[i] != want[i] {
			t.Errorf("Remotes[%d] = %v, want %v", i, info.Remotes[i], want[i])
		}
	}
}

func TestInspectDefaultBranch(t *testing.T) {
	tests := map[string]struct {
		setup       func(t *testing.T, dir string)
		wantDefault string
		wantCurrent string
	}{
		"main wins over master": {
			setup:       func(t *testing.T, dir string) { git(t, dir, "branch", "master") },
			wantDefault: "main", wantCurrent: "main",
		},
		"master when there is no main": {
			setup: func(t *testing.T, dir string) {
				git(t, dir, "branch", "-m", "main", "master")
			},
			wantDefault: "master", wantCurrent: "master",
		},
		"the current branch when there is neither": {
			setup: func(t *testing.T, dir string) {
				git(t, dir, "branch", "-m", "main", "trunk")
			},
			wantDefault: "trunk", wantCurrent: "trunk",
		},
		"origin HEAD wins over main": {
			setup: func(t *testing.T, dir string) {
				git(t, dir, "update-ref", "refs/remotes/origin/develop", "HEAD")
				git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")
			},
			wantDefault: "develop", wantCurrent: "main",
		},
		"a detached HEAD has no current branch": {
			setup:       func(t *testing.T, dir string) { git(t, dir, "checkout", "--quiet", "--detach") },
			wantDefault: "main", wantCurrent: "",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := fixture(t, "small-repo")
			tc.setup(t, dir)
			info, err := testGit().Inspect(context.Background(), dir)
			if err != nil {
				t.Fatal(err)
			}
			if info.DefaultBranch != tc.wantDefault || info.CurrentBranch != tc.wantCurrent {
				t.Errorf("default, current = %q, %q; want %q, %q",
					info.DefaultBranch, info.CurrentBranch, tc.wantDefault, tc.wantCurrent)
			}
		})
	}
}

func TestInspectAllowsARepoWithNoCommits(t *testing.T) {
	dir := newEmptyRepo(t)
	info, err := testGit().Inspect(context.Background(), dir)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.HasCommits {
		t.Error("HasCommits = true for a repository with no commits")
	}
	if info.CurrentBranch != "main" || info.DefaultBranch != "main" || !info.Clean {
		t.Errorf("info = %+v, want the unborn branch main and a clean tree", info)
	}
}

func TestInspectStopsWhenTheContextIsCancelled(t *testing.T) {
	dir := fixture(t, "small-repo")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := testGit().Inspect(ctx, dir)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Inspect = %v, want a cancel", err)
	}
	if errors.Is(err, gitx.ErrNotARepo) {
		t.Error("a cancel was reported as a folder that is not a repository")
	}
}
