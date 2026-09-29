package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// programProbe finds programs from a table, by name, so no machine's own tools are looked at.
type programProbe struct{ found map[string]Found }

func (p programProbe) Probe(_ context.Context, kind protocol.AgentKind) (Found, error) {
	return p.ProbeProgram(context.Background(), specFor(kind).program)
}

func (p programProbe) ProbeProgram(_ context.Context, program string) (Found, error) {
	f, ok := p.found[program]
	if !ok {
		return Found{}, ErrNotFound
	}
	return f, nil
}

func ver(major, minor, patch int) Semver { return Semver{Major: major, Minor: minor, Patch: patch} }

func TestOnlyInstalledToolsAreListedWithHowTheyTakeWork(t *testing.T) {
	c := New(Options{Probe: programProbe{found: map[string]Found{
		"qwen": {Path: "/x/qwen", Version: ver(0, 15, 6)},
		"pi":   {Path: "/x/pi", Version: ver(0, 70, 6)},
	}}})
	list, err := c.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 2 || list.Tools[0].ID != "qwen" || list.Tools[1].ID != "pi" {
		t.Fatalf("tools = %+v", list.Tools)
	}
	if list.Tools[0].Interface != protocol.AgentToolInterfaceACP || list.Tools[1].Interface != protocol.AgentToolInterfaceRPC {
		t.Fatalf("interfaces = %+v", list.Tools)
	}
	if list.Tools[0].Version != "0.15.6" {
		t.Fatalf("version = %q", list.Tools[0].Version)
	}
}

func TestNoToolsIsAnEmptyListNotNull(t *testing.T) {
	list, err := New(Options{Probe: programProbe{}}).List(t.Context())
	if err != nil || list.Tools == nil || len(list.Tools) != 0 {
		t.Fatalf("tools = %#v, %v", list.Tools, err)
	}
}

func TestTestingAnUnknownAgentIsRefused(t *testing.T) {
	_, err := New(Options{Probe: programProbe{}}).Test(t.Context(), "nope")
	if !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("err = %v", err)
	}
}

func TestAMissingAgentFailsItsTestWithHowToInstallIt(t *testing.T) {
	got, err := New(Options{Probe: programProbe{}}).Test(t.Context(), "codex")
	if err != nil || got.OK {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got.Checks[0].State != protocol.CheckStateFailed || got.Checks[0].Fix == "" {
		t.Fatalf("checks = %+v", got.Checks)
	}
}

// writeScript makes a small executable that stands in for an agent program.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts")
	}
	path := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnACPServerIsGreetedAndItsAnswerIsRead(t *testing.T) {
	path := writeScript(t, `echo "banner line"
read line
echo '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentInfo":{"name":"fake-agent","version":"9.9.9"}}}'
sleep 1`)
	got, err := acpHello(t.Context(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fake-agent 9.9.9 answered the greeting and speaks protocol version 1." {
		t.Fatalf("got %q", got)
	}
}

func TestAnACPServerThatAnswersWithAnErrorFails(t *testing.T) {
	path := writeScript(t, `read line
echo '{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"no such method"}}'`)
	if _, err := acpHello(t.Context(), path, nil); err == nil {
		t.Fatal("an error answer counted as a greeting")
	}
}

func TestAnACPServerThatNeverAnswersTimesOut(t *testing.T) {
	path := writeScript(t, `sleep 5`)
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	if _, err := acpHello(ctx, path, nil); err == nil {
		t.Fatal("a silent server passed")
	}
}

func TestAProgramThatPrintsItsVersionOnStandardErrorIsRead(t *testing.T) {
	path := writeScript(t, `echo "0.70.6" 1>&2`)
	v, err := NewProbe(ProbeOptions{}).readVersion(t.Context(), path)
	if err != nil || v.String() != "0.70.6" {
		t.Fatalf("version = %v, %v", v, err)
	}
}
