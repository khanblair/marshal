package testutil_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestFixturesLoadAsCommittedRepositories(t *testing.T) {
	for name, wantFile := range map[string]string{
		"small-repo": filepath.Join("src", "util.js"),
		"monorepo":   filepath.Join("packages", "api", "package.json"),
	} {
		t.Run(name, func(t *testing.T) {
			dir := testutil.Fixture(t, name)
			if _, err := os.Stat(filepath.Join(dir, wantFile)); err != nil {
				t.Fatalf("fixture is missing %s: %v", wantFile, err)
			}
			git := gitx.New()
			branch, err := git.Run(context.Background(), dir, "branch", "--show-current")
			if err != nil || branch != "main" {
				t.Fatalf("branch = %q, %v; want main", branch, err)
			}
			log, err := git.Run(context.Background(), dir, "log", "--format=%an|%s")
			if err != nil || !strings.Contains(log, "Marshal Test|Fixture: "+name) {
				t.Fatalf("log = %q, %v", log, err)
			}
			status, _ := git.Run(context.Background(), dir, "status", "--porcelain")
			if status != "" {
				t.Errorf("the fixture repo is not clean: %q", status)
			}
		})
	}
}

func TestFixturesAreCopiedNotChanged(t *testing.T) {
	first := testutil.Fixture(t, "small-repo")
	if err := os.WriteFile(filepath.Join(first, "README.md"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := testutil.Fixture(t, "small-repo")
	data, err := os.ReadFile(filepath.Join(second, "README.md"))
	if err != nil || string(data) == "changed" {
		t.Errorf("a change leaked between fixture copies: %q, %v", data, err)
	}
}
