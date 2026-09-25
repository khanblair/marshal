package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// fakeEnv switches this test binary into the fake claude program (see main_test.go). Its value is
// a comma separated list of behaviors:
//
//   - "crash": exit with a non-zero code before any output, so Start fails.
//   - "noready": never emit the "init" line, so Start becomes ready only through Config.ReadyWindow.
//   - "lateinit": emit "init" only once the first user message arrives, not right away, to prove
//     Start does not depend on which order Claude Code actually sends it in.
//   - "badid": emit "init" with a session id that does not match what was asked for.
//   - "crashmidturn": exit with no result line partway through a turn.
//   - "ignoreinterrupt": never answer a control_request, so the interrupt grace timer runs out.
//   - "ignoresigint": receive SIGINT (which this fake otherwise answers the documented way, by
//     ending the turn) but do nothing with it, so the second grace timer runs out too.
const fakeEnv = "MARSHAL_FAKE_CLAUDE"

// toolCallSeq numbers the fake tool calls it announces, across every session in the test binary.
var toolCallSeq atomic.Uint64

// stdoutMu serializes writes to standard output: the main read loop and the SIGINT handler below
// both write lines, and a real process's stdout is not safe for concurrent writers.
var stdoutMu sync.Mutex

// runFakeClaude serves one scripted "claude -p --input-format stream-json" session on standard
// input and output: an init line, then for every user message a streamed answer, a tool call, and
// a result, and a control_response to an interrupt request, unless the flags say otherwise. It
// never talks to the network or a real model; it only echoes back what it was asked for, so tests
// can check that this adapter sent the arguments and the stdin lines it meant to.
func runFakeClaude(spec string) {
	flags := fakeFlags(spec)
	if flags["crash"] {
		fmt.Fprintln(os.Stderr, "fake claude: told to crash before starting")
		os.Exit(7)
	}
	id := sessionIDFromArgs(os.Args[1:])
	pending := flags["lateinit"]
	if !flags["noready"] && !pending {
		writeInit(id, flags["badid"])
	}
	watchSigint(flags)
	fc := fakeContext{
		flags: flags, id: id, resumed: isResume(os.Args[1:]),
		argsLine: fmt.Sprintf("pid=%d args:%s", os.Getpid(), strings.Join(os.Args[1:], "|")),
	}
	serve(fc, pending)
}

// fakeContext is what every line this fake answers needs to know about the process it is
// running as, gathered once in runFakeClaude so the functions below take one argument for it
// instead of four.
type fakeContext struct {
	flags    map[string]bool
	id       string
	argsLine string
	resumed  bool
}

// watchSigint answers SIGINT the way Claude Code's own documentation says a real claude -p
// process does: it ends the turn in flight, without exiting, unless told to ignore it (which
// still claims the signal, so the process does not fall back to Go's default of exiting on it;
// it simulates a Claude Code that stays alive but does not honor SIGINT as documented).
func watchSigint(flags map[string]bool) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		for range ch {
			if !flags["ignoresigint"] {
				writeLine(resultOf("interrupted by sigint", false, "success", "end_turn"))
			}
		}
	}()
}

// isResume says whether the process was started with --resume rather than --session-id, so the
// fake can behave as though it recovered from whatever earlier flags (such as "hold") made the
// first process misbehave.
func isResume(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "--resume=") {
			return true
		}
	}
	return false
}

// fakeFlags turns fakeEnv's comma separated value into a set.
func fakeFlags(spec string) map[string]bool {
	flags := make(map[string]bool)
	for _, name := range strings.Split(spec, ",") {
		if name != "" {
			flags[name] = true
		}
	}
	return flags
}

// sessionIDFromArgs reads the id that --session-id or --resume was given.
func sessionIDFromArgs(args []string) string {
	for _, a := range args {
		for _, prefix := range []string{"--session-id=", "--resume="} {
			if v, ok := strings.CutPrefix(a, prefix); ok {
				return v
			}
		}
	}
	return ""
}

// writeInit writes the "init" line that a real claude -p session starts with.
func writeInit(id string, wrongID bool) {
	reported := id
	if wrongID {
		reported = "00000000-0000-4000-8000-000000000000"
	}
	writeLine(map[string]any{"type": "system", "subtype": "init", "session_id": reported})
}

// serve reads stream-json input lines until standard input closes, and answers each one. The
// reading itself is single threaded, on purpose: every write from it finishes before the next
// line is read, so a test never has to guess at ordering (watchSigint's writes are the one
// exception, guarded by stdoutMu). pendingInit says the "init" line has not been sent yet; the
// first user message triggers it (flag "lateinit").
func serve(fc fakeContext, pendingInit bool) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	turnNum := 0
	for scanner.Scan() {
		var head fakeInLine
		if err := json.Unmarshal(scanner.Bytes(), &head); err != nil {
			continue
		}
		switch head.Type {
		case "user":
			turnNum++
			if pendingInit {
				writeInit(fc.id, fc.flags["badid"])
				pendingInit = false
			}
			handleTurn(fc, head, turnNum == 1)
		case "control_request":
			handleControlRequest(fc.flags, head.RequestID, head.Request.Subtype)
		}
	}
}

// fakeInLine reads the union of everything this fake needs from an input line.
type fakeInLine struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Request   struct {
		Subtype string `json:"subtype"`
	} `json:"request"`
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

// handleTurn answers one user message: a streamed echo, that same text as a complete message (so
// a test can check it is not counted twice), a tool call and its result, and finally the line
// that ends the turn. Flag "hold" skips the tool call and the result on the first turn of a
// process that was not itself started with --resume, so that turn only ends through a
// control_request or a signal; every later turn on the same process, and every turn once the
// process has been resumed, finishes normally, the way a real Claude Code would once the one
// stuck turn is dealt with. Flag "showargs" streams the process's own argument list (with its
// pid) as a third chunk, for a test that checks what this adapter passed on the command line and
// whether a later turn answered on the same process or a fresh one; it is a separate flag so the
// ordinary happy-path tests keep a clean, predictable answer.
func handleTurn(fc fakeContext, in fakeInLine, firstTurn bool) {
	answer := "echo: " + in.Message.Content
	writeLine(streamDelta("text_delta", "echo: "))
	writeLine(streamDelta("text_delta", in.Message.Content))
	if fc.flags["showargs"] {
		writeLine(streamDelta("text_delta", " "+fc.argsLine))
	}
	writeLine(assistantText(answer))
	if fc.flags["crashmidturn"] {
		fmt.Fprintln(os.Stderr, "fake claude: told to crash mid-turn")
		os.Exit(9)
	}
	if fc.flags["hold"] && !fc.resumed && firstTurn {
		return
	}
	id := "call-" + strconv.FormatUint(toolCallSeq.Add(1), 10)
	writeLine(assistantToolUse(id, "Bash", map[string]any{"command": "echo hi"}))
	writeLine(userToolResult(id, "hi\n", fc.flags["toolfails"]))
	if fc.flags["failresult"] {
		writeLine(resultOf("could not finish", true, "error_during_execution", "tool_use"))
		return
	}
	writeLine(resultOf(answer, false, "success", "end_turn"))
}

// handleControlRequest answers an interrupt request, unless the fake was told to ignore it (to
// exercise the interrupt grace timer's fallback).
func handleControlRequest(flags map[string]bool, requestID, subtype string) {
	if subtype != "interrupt" || flags["ignoreinterrupt"] {
		return
	}
	writeLine(map[string]any{
		"type":     "control_response",
		"response": map[string]any{"request_id": requestID, "subtype": "success"},
	})
	writeLine(resultOf("interrupted", false, "success", "end_turn"))
}

// streamDelta is one stream_event content_block_delta line.
func streamDelta(deltaType, text string) map[string]any {
	return map[string]any{
		"type": "stream_event",
		"event": map[string]any{
			"type":  "content_block_delta",
			"delta": map[string]any{"type": deltaType, "text": text},
		},
	}
}

// assistantText is a complete assistant message holding one text block.
func assistantText(text string) map[string]any {
	return map[string]any{
		"type":    "assistant",
		"message": map[string]any{"content": []map[string]any{{"type": "text", "text": text}}},
	}
}

// assistantToolUse is a complete assistant message holding one tool_use block.
func assistantToolUse(id, name string, input map[string]any) map[string]any {
	return map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []map[string]any{{"type": "tool_use", "id": id, "name": name, "input": input}},
		},
	}
}

// userToolResult is the echo of a tool's result, the way Claude Code sends it back as a "user"
// line.
func userToolResult(id, content string, isError bool) map[string]any {
	return map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": []map[string]any{
				{"type": "tool_result", "tool_use_id": id, "content": content, "is_error": isError},
			},
		},
	}
}

// resultOf is the "result" line that ends a turn.
func resultOf(result string, isError bool, subtype, stopReason string) map[string]any {
	return map[string]any{
		"type": "result", "subtype": subtype, "is_error": isError, "result": result, "stop_reason": stopReason,
	}
}

// writeLine writes one JSON value as a line of stream-json output.
func writeLine(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake claude: could not build a line:", err)
		return
	}
	stdoutMu.Lock()
	defer stdoutMu.Unlock()
	if _, err := os.Stdout.Write(append(b, '\n')); err != nil {
		// The reader is gone (the adapter stopped this process); there is nobody left to tell.
		os.Exit(1)
	}
}
