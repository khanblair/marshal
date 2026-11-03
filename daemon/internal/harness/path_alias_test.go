package harness_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

func TestFolderAliasesAndNewFilesStayContained(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(project, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	c := gitx.NewContainment(alias, alias, "", "")
	if !harness.InsideWorktree(c, filepath.Join(project, "new", "file.txt")) {
		t.Fatal("canonical path inside aliased project refused")
	}
	if err := os.Symlink(outside, filepath.Join(project, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "missing"), filepath.Join(project, "dangling")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"escape/new.txt", filepath.Join(alias, "escape", "new.txt"), "../outside/new.txt", "dangling/new.txt", "escape/../new.txt"} {
		if harness.InsideWorktree(c, path) {
			t.Fatalf("outside path allowed: %s", path)
		}
	}
}

func TestEveryFileInAnApprovalMustBeContained(t *testing.T) {
	root := t.TempDir()
	c := gitx.NewContainment(root, root, "", "")
	for _, mode := range []protocol.PermissionMode{protocol.PermissionModeFullAuto, protocol.PermissionModeBypass} {
		cfg := harness.Config{Mode: mode, Profile: security.DefaultProfile(), Containment: &c}
		request := harness.Request{Kind: "other", Paths: []string{"inside.txt", filepath.Join(root, "..", "outside.txt")}}
		if got := cfg.Decide(request); got.Decision != harness.DecisionDeny {
			t.Fatalf("%s allowed an outside file in a grouped edit: %+v", mode, got)
		}
		request.Paths = []string{"inside.txt", "nested/new.txt"}
		if got := cfg.Decide(request); got.Decision != harness.DecisionAllow {
			t.Fatalf("%s refused contained grouped edit: %+v", mode, got)
		}
	}
}
