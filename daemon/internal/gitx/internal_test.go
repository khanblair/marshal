package gitx

// These tests reach into the package for the parts that are hard to trigger from outside: what
// the parsers do with odd input, the arguments passed to Git, and the clean up after a failure.
// They cannot use testutil, which imports this package.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func isolatedGit() *Git {
	return New(WithEnv(
		"GIT_AUTHOR_NAME=Marshal Test", "GIT_AUTHOR_EMAIL=test@marshal.invalid",
		"GIT_COMMITTER_NAME=Marshal Test", "GIT_COMMITTER_EMAIL=test@marshal.invalid",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
	))
}

// monorepo makes a repository with two packages and one commit on main.
func monorepo(t *testing.T, g *Git) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	for _, name := range []string{"packages/api/index.js", "packages/web/index.js", "package.json"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"add", "--all"},
		{"-c", "commit.gpgsign=false", "commit", "--quiet", "--message", "start"},
	} {
		if _, err := g.Run(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestParseWorktrees(t *testing.T) {
	out := strings.Join([]string{
		"worktree /home/me/app", "HEAD 1111111111111111111111111111111111111111", "branch refs/heads/main", "",
		"worktree /data/worktrees/acme/card with space", "HEAD 2222222222222222222222222222222222222222",
		"branch refs/heads/marshal/acme-1-work", "locked in use by an agent", "",
		"worktree /data/worktrees/acme/gone", "HEAD 3333333333333333333333333333333333333333", "detached",
		"prunable gitdir file points to non-existent location", "",
		"worktree /home/me/app.git", "bare", "", "",
	}, "\x00")
	want := []Worktree{
		{Path: filepath.FromSlash("/home/me/app"), Head: strings.Repeat("1", 40), Branch: "main"},
		{Path: filepath.FromSlash("/data/worktrees/acme/card with space"), Head: strings.Repeat("2", 40),
			Branch: "marshal/acme-1-work", Locked: true},
		{Path: filepath.FromSlash("/data/worktrees/acme/gone"), Head: strings.Repeat("3", 40), Detached: true, Prunable: true},
		{Path: filepath.FromSlash("/home/me/app.git"), Bare: true},
	}
	if got := parseWorktrees(out); !reflect.DeepEqual(got, want) {
		t.Errorf("parseWorktrees =\n%+v\nwant\n%+v", got, want)
	}
	if got := parseWorktrees(""); len(got) != 0 {
		t.Errorf("parseWorktrees(empty) = %+v, want none", got)
	}
	if got := parseWorktrees("HEAD abc\x00branch refs/heads/x\x00"); len(got) != 0 {
		t.Errorf("lines before any worktree line were kept: %+v", got)
	}
}

func TestCheckAbsolute(t *testing.T) {
	base := t.TempDir()
	sep := string(os.PathSeparator)
	tests := []struct {
		name string
		path string
		want error
	}{
		{"a full path", base, nil},
		{"a trailing separator", base + sep, nil},
		{"a dot segment", base + sep + "." + sep + "x", nil},
		{"a relative path", "a" + sep + "b", ErrBadPath},
		{"an empty path", "", ErrBadPath},
		{"a dot dot in the middle", base + sep + "a" + sep + ".." + sep + "b", ErrBadPath},
		{"a dot dot at the end", base + sep + "a" + sep + "..", ErrBadPath},
		{"a dot dot with slashes", base + "/a/../b", ErrBadPath},
		{"a name that only starts with dots", base + sep + "..hidden", nil},
	}
	for _, tc := range tests {
		got, err := checkAbsolute(tc.path)
		if (tc.want == nil && err != nil) || (tc.want != nil && !errors.Is(err, tc.want)) {
			t.Errorf("%s: checkAbsolute(%q) = %q, %v; want %v", tc.name, tc.path, got, err, tc.want)
		}
		if err == nil && (got != filepath.Clean(tc.path)) {
			t.Errorf("%s: checkAbsolute(%q) = %q, want it cleaned", tc.name, tc.path, got)
		}
	}
}

func TestRelationOf(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "root")
	if err := os.MkdirAll(filepath.Join(parent, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(parent+"-evil", 0o755); err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		child string
		want  relation
	}{
		"the same folder":               {parent, sameFolder},
		"the same with a separator":     {parent + string(os.PathSeparator), sameFolder},
		"a folder inside":               {filepath.Join(parent, "a"), insideFolder},
		"a folder deeper":               {filepath.Join(parent, "a", "b"), insideFolder},
		"a folder that does not exist":  {filepath.Join(parent, "x", "y", "z"), insideFolder},
		"a sibling with the same start": {parent + "-evil", unrelated},
		"the parent of the folder":      {base, unrelated},
		"a new folder somewhere else":   {filepath.Join(base, "nowhere", "x"), unrelated},
		"a dot dot that leaves":         {filepath.Join(parent, "a") + "/../../root-evil", unrelated},
	}
	for name, tc := range tests {
		if got := relationOf(parent, tc.child); got != tc.want {
			t.Errorf("%s: relationOf(root, %q) = %d, want %d", name, tc.child, got, tc.want)
		}
	}
}

func TestRelationOfFollowsLinks(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "root")
	elsewhere := filepath.Join(base, "elsewhere")
	for _, dir := range []string{filepath.Join(parent, "a"), elsewhere} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	toParent := filepath.Join(base, "to-root")
	toElsewhere := filepath.Join(parent, "leaves")
	if err := os.Symlink(parent, toParent); err != nil {
		t.Skipf("this machine cannot make symbolic links: %v", err)
	}
	if err := os.Symlink(elsewhere, toElsewhere); err != nil {
		t.Skipf("this machine cannot make symbolic links: %v", err)
	}
	tests := map[string]struct {
		child string
		want  relation
	}{
		"inside, through a link to the root":     {filepath.Join(toParent, "a"), insideFolder},
		"new folder, through a link to the root": {filepath.Join(toParent, "new"), insideFolder},
		"the root, through a link":               {toParent, sameFolder},
		"a link inside that leaves":              {toElsewhere, unrelated},
		"a new folder under a link that leaves":  {filepath.Join(toElsewhere, "new"), unrelated},
	}
	for name, tc := range tests {
		if got := relationOf(parent, tc.child); got != tc.want {
			t.Errorf("%s: relationOf(root, %q) = %d, want %d", name, tc.child, got, tc.want)
		}
	}
}

func TestCloneArgsKeepTheAddressAfterTheDashes(t *testing.T) {
	u := CloneURL{Raw: "https://example.com/acme/web.git"}
	args := cloneArgs(u, "/data/web", CloneOptions{Branch: "release"})
	dashes := slices.Index(args, "--")
	if dashes < 0 || !reflect.DeepEqual(args[dashes:], []string{"--", u.Raw, "/data/web"}) {
		t.Fatalf("args = %v, want the address and the folder after --", args)
	}
	for _, setting := range []string{
		"protocol.allow=never", "protocol.ext.allow=never", "protocol.https.allow=always",
		"protocol.ssh.allow=always", "protocol.file.allow=never",
	} {
		if !slices.Contains(args[:dashes], setting) {
			t.Errorf("args = %v, missing -c %s", args, setting)
		}
	}
	if i := slices.Index(args, "clone"); i < 0 || i > dashes {
		t.Errorf("the settings must come before clone: %v", args)
	}
	if slices.Contains(args, "credential.helper=") {
		t.Errorf("an address without credentials should leave the credential helpers alone: %v", args)
	}
	secret := cloneArgs(CloneURL{Raw: "https://tok@example.com/x.git", HasCredentials: true}, "/data/web", CloneOptions{})
	i := slices.Index(secret, "credential.helper=")
	if i < 1 || secret[i-1] != "-c" || i > slices.Index(secret, "clone") {
		t.Errorf("an address with credentials must clear the credential helpers before clone: %v", secret)
	}
	if !slices.Contains(args, "--branch") {
		t.Errorf("args = %v, want --branch", args)
	}
	allowed := cloneArgs(u, "/data/web", CloneOptions{AllowLocal: true})
	if !slices.Contains(allowed, "protocol.file.allow=always") || slices.Contains(allowed, "protocol.file.allow=never") {
		t.Errorf("AllowLocal did not allow the file protocol: %v", allowed)
	}
}

func TestRedactErrorRemovesCredentialsEverywhere(t *testing.T) {
	u := CloneURL{
		Raw:      "https://someone:s3cr3t@example.com/acme/web.git",
		Redacted: "https://example.com/acme/web.git",
	}
	cause := &Error{
		Args:   []string{"clone", "--", u.Raw, "/data/web"},
		Stderr: "fatal: unable to access '" + u.Raw + "': failed\nretry https://other:hunter2@elsewhere.example/x.git",
		Err:    errors.New("exit status 128"),
	}
	got := redactError(cause, u)
	var gitErr *Error
	if !errors.As(got, &gitErr) {
		t.Fatalf("redactError = %v, want a *Error", got)
	}
	whole := got.Error() + strings.Join(gitErr.Args, " ") + gitErr.Stderr
	for _, secret := range []string{"s3cr3t", "someone", "hunter2", "other:"} {
		if strings.Contains(whole, secret) {
			t.Errorf("the redacted error still has %q in it: %s", secret, whole)
		}
	}
	if !strings.Contains(whole, u.Redacted) {
		t.Errorf("the redacted error lost the address: %s", whole)
	}
	if cause.Args[2] != u.Raw {
		t.Error("redactError changed the original error")
	}
	plain := errors.New("no address in here")
	if redactError(plain, u) != plain {
		t.Error("an error that is not from Git was changed")
	}
}

func TestFinishCloneRemovesCredentialsFromTheStoredAddress(t *testing.T) {
	ctx := context.Background()
	g := isolatedGit()
	dir := monorepo(t, g)
	secret := "https://someone:s3cr3t@example.com/acme/web.git"
	if _, err := g.Run(ctx, dir, "remote", "add", "origin", secret); err != nil {
		t.Fatal(err)
	}
	u := CloneURL{Raw: secret, Redacted: "https://example.com/acme/web.git", HasCredentials: true}
	if err := g.finishClone(ctx, u, dir); err != nil {
		t.Fatalf("finishClone: %v", err)
	}
	got, err := g.Run(ctx, dir, "config", "--get", "remote.origin.url")
	if err != nil || got != u.Redacted {
		t.Errorf("origin = %q, %v; want %q", got, err, u.Redacted)
	}
	config, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil || strings.Contains(string(config), "s3cr3t") {
		t.Errorf("the repository config still holds the secret: %v", err)
	}
}

func TestLimitToFoldersNeverChecksOutAfterAFailure(t *testing.T) {
	ctx := context.Background()
	g := isolatedGit()
	repo := monorepo(t, g)
	wt := filepath.Join(t.TempDir(), "wt")
	if _, err := g.Run(ctx, repo, "worktree", "add", "--no-checkout", "-b", "work", "--", wt, "main"); err != nil {
		t.Fatal(err)
	}
	// Git refuses a folder name with a pattern character in it. The checks in front of this
	// function keep such names out, so it is fed one directly.
	if err := g.limitToFolders(ctx, wt, []string{"packages/a*"}); err == nil {
		t.Fatal("limitToFolders accepted a pattern")
	}
	entries, err := os.ReadDir(wt)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != ".git" {
			t.Errorf("%s was checked out after the limit failed, which would have written the whole tree", e.Name())
		}
	}
}

// wrapperGit returns a Git that runs a script instead of Git. The script writes each call to a
// log, and fails any call that has a word from failOn among its arguments, so a test can make one
// step of a longer job fail.
func wrapperGit(t *testing.T, failOn ...string) (*Git, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the wrapper is a shell script")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> \"$WRAPPER_LOG\"\n" +
		"for arg in \"$@\"; do\n" +
		"  case \"$arg\" in\n"
	for _, word := range failOn {
		script += "    " + word + ") echo 'fatal: simulated failure' >&2; exit 128;;\n"
	}
	script += "  esac\ndone\nexec git \"$@\"\n"
	bin := filepath.Join(dir, "git-wrapper")
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	g := isolatedGit()
	g.bin = bin
	g.env = append(g.env, "WRAPPER_LOG="+log)
	return g, log
}

func TestAddWorktreeIsRolledBackWhenTheSparseStepFails(t *testing.T) {
	ctx := context.Background()
	g, log := wrapperGit(t, "sparse-checkout")
	repo := monorepo(t, isolatedGit())
	wt := filepath.Join(t.TempDir(), "wt")
	spec := WorktreeSpec{Path: wt, Branch: "marshal/x-1-work", Base: "main", Sparse: []string{"packages/api"}}

	err := g.AddWorktree(ctx, repo, spec)
	var gitErr *Error
	if !errors.As(err, &gitErr) || !strings.Contains(gitErr.Stderr, "simulated failure") {
		t.Fatalf("AddWorktree = %v, want the simulated Git failure", err)
	}
	if _, statErr := os.Stat(wt); !os.IsNotExist(statErr) {
		t.Error("the worktree folder is still there")
	}
	if found, _ := g.BranchExists(ctx, repo, spec.Branch); found {
		t.Error("the branch is still there")
	}
	if list, _ := g.ListWorktrees(ctx, repo); len(list) != 1 {
		t.Errorf("worktrees = %+v, want only the main one", list)
	}
	calls, readErr := os.ReadFile(log)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(calls), "read-tree") {
		t.Error("the tree was checked out after the sparse step failed, which writes every folder")
	}
}

func TestAddWorktreeRollbackReportsWhatItCouldNotUndo(t *testing.T) {
	ctx := context.Background()
	g, _ := wrapperGit(t, "sparse-checkout", "prune")
	repo := monorepo(t, isolatedGit())
	spec := WorktreeSpec{Path: filepath.Join(t.TempDir(), "wt"), Branch: "work", Base: "main", Sparse: []string{"packages/api"}}
	err := g.AddWorktree(ctx, repo, spec)
	if err == nil || strings.Count(err.Error(), "simulated failure") < 2 {
		t.Errorf("AddWorktree = %v, want both the failure and the failed clean up", err)
	}
}
