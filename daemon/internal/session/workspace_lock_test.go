package session

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/platform"
)

func TestWorkspaceLockAliasesShareOneLease(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("make a symlink alias: %v", err)
	}
	rootIdentity, err := workspaceIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	aliasIdentity, err := workspaceIdentity(alias)
	if err != nil {
		t.Fatal(err)
	}
	if rootIdentity != aliasIdentity {
		t.Fatalf("physical folder identities differ: %q != %q", rootIdentity, aliasIdentity)
	}
	if runtime.GOOS == "darwin" {
		caseAlias := strings.ToUpper(root)
		rootInfo, rootErr := os.Stat(root)
		aliasInfo, aliasErr := os.Stat(caseAlias)
		if rootErr == nil && aliasErr == nil && os.SameFile(rootInfo, aliasInfo) {
			caseIdentity, err := workspaceIdentity(caseAlias)
			if err != nil {
				t.Fatal(err)
			}
			if rootIdentity != caseIdentity {
				t.Fatalf("case alias physical folder identity differs: %q != %q", rootIdentity, caseIdentity)
			}
		}
	}

	lockDir := filepath.Join(t.TempDir(), "locks")
	first, err := acquireWorkspaceLock(rootIdentity, lockDir)
	if err != nil {
		t.Fatalf("acquire the project lease: %v", err)
	}
	defer func() {
		if err := first.Release(); err != nil {
			t.Errorf("release the project lease: %v", err)
		}
	}()
	if _, err := acquireWorkspaceLock(aliasIdentity, lockDir); !errors.Is(err, platform.ErrAlreadyRunning) {
		t.Fatalf("alias lease error = %v, want ErrAlreadyRunning", err)
	}
}

// Allow the race-instrumented test executable to start on a busy CI runner.
const workspaceLeaseChildTimeout = 45 * time.Second

// The installed daemon and dev daemon are separate processes, so contention must work
// beyond a single Manager map. A crashed owner must release its OS lease as well.
func TestWorkspaceLeaseSurvivesAcrossProcessesAndReleasesAfterCrash(t *testing.T) {
	identity, err := workspaceIdentity(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lockDir := filepath.Join(t.TempDir(), "locks")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), workspaceLeaseChildTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestWorkspaceLeaseChild$")
	command.Env = append(os.Environ(), "MARSHAL_LOCK_TEST_ID="+identity, "MARSHAL_LOCK_TEST_DIR="+lockDir)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "locked\n" {
		_ = command.Process.Kill()
		waitErr := command.Wait()
		t.Fatalf("child did not acquire lease: stdout %q, read %v, context %v, exit %v, stderr %q",
			line, err, ctx.Err(), waitErr, stderr.String())
	}
	if _, err := acquireWorkspaceLock(identity, lockDir); !errors.Is(err, platform.ErrAlreadyRunning) {
		t.Fatalf("second process acquired owned lease: %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	lease, err := acquireWorkspaceLock(identity, lockDir)
	if err != nil {
		t.Fatalf("crashed process retained lease: %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceLeaseChild(t *testing.T) {
	identity := os.Getenv("MARSHAL_LOCK_TEST_ID")
	if identity == "" {
		return
	}
	lease, err := acquireWorkspaceLock(identity, os.Getenv("MARSHAL_LOCK_TEST_DIR"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lease.Release() }()
	_, _ = fmt.Println("locked")
	_, _ = io.Copy(io.Discard, os.Stdin)
}
