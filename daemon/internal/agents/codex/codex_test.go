package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

func TestCodexProcess(t *testing.T) {
	if os.Getenv("MARSHAL_CODEX_HELPER") != "1" {
		return
	}
	scan := bufio.NewScanner(os.Stdin)
	scan.Buffer(make([]byte, 4096), 8<<20)
	out := bufio.NewWriter(os.Stdout)
	mode := os.Getenv("CODEX_FIXTURE_MODE")
	for scan.Scan() {
		var m map[string]json.RawMessage
		_ = json.Unmarshal(scan.Bytes(), &m)
		var method string
		_ = json.Unmarshal(m["method"], &method)
		id := m["id"]
		if method == "initialize" && mode == "hang-init" {
			continue
		}
		if method == "turn/start" && mode == "hang-turn" {
			// Model an accepted request whose response never reaches the client.
			continue
		}
		if method == "initialize" && mode == "large" {
			if _, err := fmt.Fprintln(out, strings.Repeat("x", 5<<20)); err != nil {
				os.Exit(1)
			}
			_ = out.Flush()
			continue
		}
		if method == "initialize" && mode == "malformed" {
			writeLine(out, `not-json`)
		}
		if len(id) > 0 {
			var result any = map[string]any{}
			switch method {
			case "thread/start", "thread/resume":
				threadID := "thread-1"
				var startParams struct {
					Cwd     string                     `json:"cwd"`
					Model   string                     `json:"model"`
					Policy  json.RawMessage            `json:"approvalPolicy"`
					Sandbox json.RawMessage            `json:"sandbox"`
					Config  map[string]json.RawMessage `json:"config"`
				}
				_ = json.Unmarshal(m["params"], &startParams)
				if mode == "validate-config" && (startParams.Cwd == "" || startParams.Model != "model-x" || len(startParams.Policy) == 0 || len(startParams.Sandbox) == 0 || startParams.Config["model_reasoning_effort"] == nil || startParams.Config["mcp_servers"] == nil || startParams.Config["approvalPolicy"] != nil || startParams.Config["sandbox"] != nil) {
					threadID = "bad-config"
				}
				if mode == "wrong-id" {
					threadID = "other-thread"
				}
				if mode == "missing-id" {
					result = map[string]any{}
				} else {
					result = map[string]any{"thread": map[string]any{"id": threadID}}
				}
			}
			writeReply(out, id, result)
			if method == "turn/interrupt" {
				writeLine(out, `{"jsonrpc":"2.0","method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"interrupted"}}}`)
			}
		}
		if method == "turn/start" {
			handleTurnStart(scan, out, mode)
		}
	}
	os.Exit(0)
}
func handleTurnStart(scan *bufio.Scanner, out *bufio.Writer, mode string) {
	writeLine(out, `{"jsonrpc":"2.0","method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1"}}}`)
	writeLine(out, `{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"threadId":"thread-1","turnId":"turn-1","delta":"hello"}}`)
	if isApprovalMode(mode) {
		writeApprovalFixture(scan, out, mode)
		return
	}
	if mode == "interrupt" {
		return
	}
	status, detail := "completed", ""
	if mode == "failed" {
		status, detail = "failed", `,"error":"fixture failed"`
	}
	writeLine(out, fmt.Sprintf(`{"jsonrpc":"2.0","method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":%q%s}}}`, status, detail))
}
func isApprovalMode(mode string) bool {
	return mode == "approval" || mode == "decline" || mode == "cancel-approval" || mode == "fallback-cancel" || mode == "file-approval" || mode == "resolved"
}
func writeApprovalFixture(scan *bufio.Scanner, out *bufio.Writer, mode string) {
	method, params, itemID := "item/commandExecution/requestApproval", `"command":"echo yes"`, "cmd-1"
	if mode == "file-approval" {
		method, params, itemID = "item/fileChange/requestApproval", `"reason":"Review requested"`, "file-1"
		writeLine(out, `{"jsonrpc":"2.0","method":"item/started","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"file-1","type":"fileChange","status":"inProgress","changes":[{"path":"a.txt","diff":"+ a"},{"path":"b.txt","diff":"+ b"}]}}}`)
	}
	decisions := "[\"accept\",\"decline\"]"
	if mode == "cancel-approval" {
		decisions = "[\"accept\",\"decline\",\"cancel\"]"
	}
	if mode == "fallback-cancel" {
		decisions = "[\"accept\",\"cancel\"]"
	}
	id := "\"server-1\""
	if mode == "approval" {
		id = "47"
	}
	if _, err := fmt.Fprintf(out, `{"jsonrpc":"2.0","id":%s,"method":%q,"params":{"threadId":"thread-1","turnId":"turn-1","itemId":%q,%s,"availableDecisions":%s}}`+"\n", id, method, itemID, params, decisions); err != nil {
		os.Exit(1)
	}
	_ = out.Flush()
	if mode == "resolved" {
		writeLine(out, `{"jsonrpc":"2.0","method":"serverRequest/resolved","params":{"requestId":"server-1","threadId":"thread-1"}}`)
	}
	if scan.Scan() {
		var reply map[string]json.RawMessage
		_ = json.Unmarshal(scan.Bytes(), &reply)
		var result map[string]string
		_ = json.Unmarshal(reply["result"], &result)
		writeLine(out, fmt.Sprintf(`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"threadId":"thread-1","turnId":"turn-1","delta":%q}}`, result["decision"]+":"+string(reply["id"])))
	}
}
func writeReply(w *bufio.Writer, id json.RawMessage, result any) {
	b, _ := json.Marshal(result)
	if _, err := fmt.Fprintf(w, "{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":%s}\n", id, b); err != nil {
		os.Exit(1)
	}
	_ = w.Flush()
}
func writeLine(w *bufio.Writer, v string) {
	if _, err := fmt.Fprintln(w, v); err != nil {
		os.Exit(1)
	}
	_ = w.Flush()
}
func helper(t *testing.T) string {
	t.Helper()
	path := t.TempDir() + "/codex"
	script := "#!/bin/sh\nexec '" + os.Args[0] + "' -test.run=^TestCodexProcess$\n"
	if e := os.WriteFile(path, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	return path
}
func adapter(t *testing.T, mode string, timeout time.Duration) agents.Agent {
	t.Helper()
	// Race-instrumented child startup can exceed one second while other packages
	// compile. Cancellation tests use their own short caller deadlines.
	if timeout < 5*time.Second {
		timeout = 5 * time.Second
	}
	a, e := New(Config{Path: helper(t), Env: []string{"MARSHAL_CODEX_HELPER=1", "CODEX_FIXTURE_MODE=" + mode}, RequestTimeout: timeout, StopGrace: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func spec(t *testing.T) agents.StartSpec { return agents.StartSpec{Cwd: t.TempDir()} }
func start(t *testing.T, a agents.Agent, s agents.StartSpec) agents.SessionHandle {
	t.Helper()
	h, e := a.Start(context.Background(), s)
	if e != nil {
		t.Fatal(e)
	}
	return h
}
func nextEvent(t *testing.T, ch <-chan agents.AgentEvent) agents.AgentEvent {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("event channel closed")
		}
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event")
		return nil
	}
}
func TestLifecycleAndResume(t *testing.T) {
	a := adapter(t, "validate-config", time.Second)
	s := spec(t)
	s.Model = "model-x"
	s.Thinking = "high"
	s.MCPServers = []agents.MCPServer{{Name: "fixture", Command: "/bin/true", Args: []string{"--safe"}, Env: []string{"TOKEN=must-stay-in-rpc"}}}
	h := start(t, a, s)
	if h.ID != "thread-1" {
		t.Fatalf("id %q", h.ID)
	}
	if e := a.Send(context.Background(), h, agents.UserMessage{Text: "hi"}); e != nil {
		t.Fatal(e)
	}
	events := a.Events(h)
	msg, end := false, false
	for !end {
		switch x := nextEvent(t, events).(type) {
		case agents.MessageChunk:
			msg = msg || strings.Contains(x.Text, "hello")
		case agents.TurnEnded:
			end = true
		}
	}
	if !msg {
		t.Fatal("message event missing")
	}
	if e := a.Stop(context.Background(), h); e != nil {
		t.Fatal(e)
	}
	h2, e := a.Resume(context.Background(), h.ID, s)
	if e != nil {
		t.Fatal(e)
	}
	if h2.ID != h.ID {
		t.Fatalf("resume id %q", h2.ID)
	}
	_ = a.Stop(context.Background(), h2)
}
func TestResumeRejectsMissingAndWrongIDs(t *testing.T) {
	if _, e := adapter(t, "", time.Second).Resume(context.Background(), "", spec(t)); !errors.Is(e, agents.ErrCannotResume) {
		t.Fatalf("missing id error %v", e)
	}
	for _, mode := range []string{"missing-id", "wrong-id"} {
		_, e := adapter(t, mode, time.Second).Resume(context.Background(), "thread-1", spec(t))
		if !errors.Is(e, agents.ErrCannotResume) {
			t.Fatalf("%s resume error %v, want ErrCannotResume", mode, e)
		}
	}
}
func TestApprovalPreservesRawIDsAndChoices(t *testing.T) {
	for _, tc := range []struct {
		mode, id, decision string
		cancelled          bool
	}{{"approval", "item/commandExecution/requestApproval:47", "accept", false}, {"decline", "item/commandExecution/requestApproval:\"server-1\"", "decline", false}, {"cancel-approval", "item/commandExecution/requestApproval:\"server-1\"", "decline", true}, {"fallback-cancel", "item/commandExecution/requestApproval:\"server-1\"", "cancel", false}} {
		t.Run(tc.mode, func(t *testing.T) {
			a := adapter(t, tc.mode, time.Second)
			h := start(t, a, spec(t))
			if e := a.Send(context.Background(), h, agents.UserMessage{Text: "go"}); e != nil {
				t.Fatal(e)
			}
			ch := a.Events(h)
			var req agents.PermissionRequested
			for {
				ev := nextEvent(t, ch)
				if p, ok := ev.(agents.PermissionRequested); ok {
					req = p
					break
				}
			}
			if req.RequestID != tc.id {
				t.Fatalf("request id %q", req.RequestID)
			}
			if e := a.Respond(context.Background(), h, agents.ApprovalResponse{RequestID: "wrong", OptionID: "accept"}); !errors.Is(e, agents.ErrUnknownRequest) {
				t.Fatalf("wrong id: %v", e)
			}
			cancelled := tc.cancelled
			option := "accept"
			if tc.decision == "cancel" {
				option = "cancel"
			} else if tc.decision == "decline" {
				option = "decline"
			} else if cancelled {
				option = "decline"
			}
			if e := a.Respond(context.Background(), h, agents.ApprovalResponse{RequestID: req.RequestID, OptionID: option, Cancelled: cancelled}); e != nil {
				t.Fatal(e)
			}
			ev := nextEvent(t, ch)
			msg, ok := ev.(agents.MessageChunk)
			if !ok || msg.Text != tc.decision+":"+rawRequestID(tc.id) {
				t.Fatalf("response confirmation %#v", ev)
			}
			if e := a.Respond(context.Background(), h, agents.ApprovalResponse{RequestID: req.RequestID, OptionID: option}); !errors.Is(e, agents.ErrUnknownRequest) {
				t.Fatalf("duplicate: %v", e)
			}
			_ = a.Stop(context.Background(), h)
		})
	}
}

func TestMultiFileApprovalUsesFullDescription(t *testing.T) {
	a := adapter(t, "file-approval", time.Second)
	h := start(t, a, spec(t))
	if e := a.Send(context.Background(), h, agents.UserMessage{}); e != nil {
		t.Fatal(e)
	}
	ch := a.Events(h)
	var request agents.PermissionRequested
	for {
		ev := nextEvent(t, ch)
		if r, ok := ev.(agents.PermissionRequested); ok {
			request = r
			break
		}
	}
	if request.Kind != "other" || request.Path != "" || len(request.Paths) != 2 || request.Paths[0] != "a.txt" || request.Paths[1] != "b.txt" || !strings.Contains(request.Command, "a.txt") || !strings.Contains(request.Command, "b.txt") {
		t.Fatalf("approval metadata %#v", request)
	}
	if e := a.Respond(context.Background(), h, agents.ApprovalResponse{RequestID: request.RequestID, OptionID: "accept"}); e != nil {
		t.Fatal(e)
	}
	_ = a.Stop(context.Background(), h)
}

func TestResolvedRequestEmitsCancellation(t *testing.T) {
	a := adapter(t, "resolved", time.Second)
	h := start(t, a, spec(t))
	if err := a.Send(context.Background(), h, agents.UserMessage{}); err != nil {
		t.Fatal(err)
	}
	ch := a.Events(h)
	var requestID string
	for {
		ev := nextEvent(t, ch)
		switch value := ev.(type) {
		case agents.PermissionRequested:
			requestID = value.RequestID
		case agents.PermissionCancelled:
			if value.RequestID != requestID {
				t.Fatalf("cancel id %q want %q", value.RequestID, requestID)
			}
			if err := a.Respond(context.Background(), h, agents.ApprovalResponse{RequestID: requestID, OptionID: "accept"}); !errors.Is(err, agents.ErrUnknownRequest) {
				t.Fatalf("resolved request remains answerable: %v", err)
			}
			_ = a.Stop(context.Background(), h)
			return
		}
	}
}
func rawRequestID(id string) string {
	if strings.HasSuffix(id, "47") {
		return "47"
	}
	return `"server-1"`
}
func TestFailedTurnAndInterrupt(t *testing.T) {
	t.Run("failed", func(t *testing.T) {
		a := adapter(t, "failed", time.Second)
		h := start(t, a, spec(t))
		if e := a.Send(context.Background(), h, agents.UserMessage{}); e != nil {
			t.Fatal(e)
		}
		ch := a.Events(h)
		var first agents.AgentEvent
		for {
			first = nextEvent(t, ch)
			if _, ok := first.(agents.Failed); ok {
				break
			}
		}
		if f := first.(agents.Failed); f.Message != "fixture failed" {
			t.Fatalf("failed event %#v", first)
		}
		if _, ok := nextEvent(t, ch).(agents.TurnEnded); !ok {
			t.Fatal("missing turn end")
		}
		_ = a.Stop(context.Background(), h)
	})
	t.Run("interrupt", func(t *testing.T) {
		a := adapter(t, "interrupt", time.Second)
		h := start(t, a, spec(t))
		if e := a.Send(context.Background(), h, agents.UserMessage{}); e != nil {
			t.Fatal(e)
		}
		if _, ok := nextEvent(t, a.Events(h)).(agents.MessageChunk); !ok {
			t.Fatal("message missing")
		}
		if e := a.Interrupt(context.Background(), h); e != nil {
			t.Fatal(e)
		}
		if ev := nextEvent(t, a.Events(h)); ev.(agents.TurnEnded).Reason != agents.TurnCancelled {
			t.Fatalf("interrupt end %#v", ev)
		}
		_ = a.Stop(context.Background(), h)
	})
}
func TestStartupCancellationAndOversizedStdout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, e := adapter(t, "hang-init", time.Second).Start(ctx, spec(t))
	if e == nil {
		t.Fatal("expected cancelled startup")
	}
	started := time.Now()
	_, e = adapter(t, "large", 5*time.Second).Start(context.Background(), spec(t))
	if e == nil {
		t.Fatal("oversized line accepted")
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("oversized stdout cleanup was not bounded")
	}
}
func TestMalformedStdoutIsIgnored(t *testing.T) {
	a := adapter(t, "malformed", time.Second)
	h := start(t, a, spec(t))
	if e := a.Stop(context.Background(), h); e != nil {
		t.Fatal(e)
	}
}
func TestSettingsMapping(t *testing.T) {
	for _, tc := range []struct{ mode, sandbox, approval string }{{"ask", "read-only", "untrusted"}, {"plan", "read-only", "never"}, {"auto-edits", "workspace-write", "untrusted"}, {"full-auto", "workspace-write", "never"}, {"bypass", "danger-full-access", "never"}} {
		thread, config, e := settings(agents.StartSpec{Cwd: "/tmp", PermissionMode: tc.mode, Model: "model-x", Thinking: "high"})
		if e != nil || thread["sandbox"] != tc.sandbox || thread["approvalPolicy"] != tc.approval || config["model_reasoning_effort"] != "high" || config["mcp_servers"] == nil {
			t.Fatalf("%s: thread=%#v config=%#v err=%v", tc.mode, thread, config, e)
		}
	}
}
