package testutil_test

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/proc"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.CleanStubAgent()
	os.Exit(code)
}

func TestStubAgentIsBuiltOnceAndRuns(t *testing.T) {
	path := testutil.StubAgent(t)
	if again := testutil.StubAgent(t); again != path {
		t.Errorf("a second call returned %q, want the same build %q", again, path)
	}
	p, err := proc.Start(t.Context(), proc.Spec{Path: path, Args: []string{"--version"}})
	if err != nil {
		t.Fatalf("start the stub agent: %v", err)
	}
	out, err := io.ReadAll(p.Stdout)
	if err != nil {
		t.Fatalf("read the stub agent's output: %v", err)
	}
	if exit := p.Wait(); exit.Err != nil {
		t.Fatalf("the stub agent exited badly: %+v", exit)
	}
	_ = p.Stdout.Close()
	if !strings.HasPrefix(string(out), "stub-agent ") {
		t.Errorf("--version printed %q, want it to start with %q", out, "stub-agent ")
	}
}
