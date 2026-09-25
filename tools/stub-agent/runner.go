package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/acp-go-sdk"
)

const (
	allowOptionID = "allow"
	denyOptionID  = "deny"
	dirMode       = 0o755
	fileMode      = 0o644
	// introBreak ends the turn message with a blank line, so the text that follows starts a new
	// paragraph when a client joins the message chunks.
	introBreak = "\n\n"
)

// peer is the client side of the connection, as far as a turn needs it. The real connection
// satisfies it, and so can a test double.
type peer interface {
	SessionUpdate(ctx context.Context, params acp.SessionNotification) error
	RequestPermission(
		ctx context.Context, params acp.RequestPermissionRequest,
	) (acp.RequestPermissionResponse, error)
}

// runner plays one turn of a scenario for one session.
type runner struct {
	peer  peer
	sid   acp.SessionId
	cwd   string
	speed float64
	// turn is this turn's number, counting from 1, and earlier is how many turns came before it.
	turn    int
	earlier int
	// calls counts the tool calls of this turn, so every wire id is unique in the session.
	calls int
	// said collects the agent text of this turn for the saved history.
	said []string
}

// outcome says how a list of steps finished.
type outcome struct {
	// ended is true when an end step, or a cancelled permission request, stopped the turn early.
	ended  bool
	reason acp.StopReason
}

// toolResult is what a tool call reports when it finishes.
type toolResult struct {
	status  acp.ToolCallStatus
	text    string
	content []acp.ToolCallContent
}

// play runs the turn message and then the scenario steps, and reports why the turn stopped.
func (r *runner) play(ctx context.Context, steps []step) (acp.StopReason, error) {
	intro := fmt.Sprintf("Turn %d. I remember %d earlier turns.", r.turn, r.earlier)
	if err := r.say(ctx, intro+introBreak); err != nil {
		return "", err
	}
	out, err := r.run(ctx, steps)
	if err != nil {
		return "", err
	}
	if out.ended {
		return out.reason, nil
	}
	return acp.StopReasonEndTurn, nil
}

// run plays steps in order and stops at the first end or error.
func (r *runner) run(ctx context.Context, steps []step) (outcome, error) {
	for _, s := range steps {
		out, err := r.exec(ctx, s)
		if err != nil || out.ended {
			return out, err
		}
	}
	return outcome{}, nil
}

// exec plays one step.
func (r *runner) exec(ctx context.Context, s step) (outcome, error) {
	switch s.Type {
	case stepSay:
		return outcome{}, r.say(ctx, s.Text)
	case stepPause:
		return outcome{}, r.pause(ctx, s.Ms)
	case stepTool:
		return outcome{}, r.tool(ctx, s.toolCall)
	case stepPermission:
		return r.permission(ctx, s)
	case stepRepeat:
		return r.repeat(ctx, s)
	case stepEnd:
		reason, _ := stopReasonOf(s.Reason)
		return outcome{ended: true, reason: reason}, nil
	}
	// A loaded scenario is already checked, so this only guards a step built by hand.
	return outcome{}, fmt.Errorf("unknown step type %q", s.Type)
}

// say streams one agent message chunk and keeps the text for the history.
func (r *runner) say(ctx context.Context, text string) error {
	r.said = append(r.said, text)
	return r.update(ctx, acp.UpdateAgentMessageText(text))
}

// update sends one session update to the client.
func (r *runner) update(ctx context.Context, u acp.SessionUpdate) error {
	return sendUpdate(ctx, r.peer, r.sid, u)
}

// transcript is the agent text of the turn as paragraphs, for the saved history.
func (r *runner) transcript() string {
	parts := make([]string, len(r.said))
	for i, text := range r.said {
		parts[i] = strings.TrimSpace(text)
	}
	return strings.Join(parts, "\n\n")
}

// pause waits for the scaled time, or until the turn is cancelled. The speed setting is a
// multiplier, so a test that passes a tiny value gets the same script without the waiting.
func (r *runner) pause(ctx context.Context, ms int) error {
	wait := time.Duration(float64(ms) * r.speed * float64(time.Millisecond))
	if wait <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("pause: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// repeat plays its steps count times, and stops early when they end the turn.
func (r *runner) repeat(ctx context.Context, s step) (outcome, error) {
	for range s.Count {
		out, err := r.run(ctx, s.Steps)
		if err != nil || out.ended {
			return out, err
		}
	}
	return outcome{}, nil
}

// tool shows a tool call that runs at once and then finishes.
func (r *runner) tool(ctx context.Context, t toolCall) error {
	id := r.nextCallID(t.ID)
	if err := r.update(ctx, r.startUpdate(id, t, acp.ToolCallStatusInProgress)); err != nil {
		return err
	}
	return r.finish(ctx, id, t)
}

// permission shows a tool call that waits for the client's answer. After an allow the call runs
// and the onAllow steps follow. After a deny the call fails and the onDeny steps follow.
func (r *runner) permission(ctx context.Context, s step) (outcome, error) {
	id := r.nextCallID(s.ID)
	if err := r.update(ctx, r.startUpdate(id, s.toolCall, acp.ToolCallStatusPending)); err != nil {
		return outcome{}, err
	}
	resp, err := r.peer.RequestPermission(ctx, r.permissionRequest(id, s.toolCall))
	if err != nil {
		return outcome{}, fmt.Errorf("request permission: %w", err)
	}
	if resp.Outcome.Cancelled != nil {
		return outcome{ended: true, reason: acp.StopReasonCancelled}, nil
	}
	if resp.Outcome.Selected == nil {
		return outcome{}, errors.New("permission answer has no outcome")
	}
	switch option := string(resp.Outcome.Selected.OptionId); option {
	case allowOptionID:
		if err := r.finish(ctx, id, s.toolCall); err != nil {
			return outcome{}, err
		}
		return r.run(ctx, s.OnAllow)
	case denyOptionID:
		denied := toolResult{status: acp.ToolCallStatusFailed, text: "Denied by the user."}
		if err := r.update(ctx, resultUpdate(id, denied)); err != nil {
			return outcome{}, err
		}
		return r.run(ctx, s.OnDeny)
	default:
		return outcome{}, fmt.Errorf("unknown permission option %q", option)
	}
}

// permissionRequest builds the request with one allow and one deny choice.
func (r *runner) permissionRequest(
	id acp.ToolCallId, t toolCall,
) acp.RequestPermissionRequest {
	kind, _ := toolKindOf(t.Kind)
	return acp.RequestPermissionRequest{
		SessionId: r.sid,
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: id,
			Title:      acp.Ptr(t.Title),
			Kind:       acp.Ptr(kind),
			Status:     acp.Ptr(acp.ToolCallStatusPending),
			RawInput:   rawInput(t),
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: allowOptionID},
			{Kind: acp.PermissionOptionKindRejectOnce, Name: "Deny", OptionId: denyOptionID},
		},
	}
}

// nextCallID makes a tool call id that is unique in the session: a repeated step would otherwise
// send the same id again and look like one call that is updated many times.
func (r *runner) nextCallID(label string) acp.ToolCallId {
	r.calls++
	return acp.ToolCallId(fmt.Sprintf("%s-t%d-%d", label, r.turn, r.calls))
}

// startUpdate announces a tool call. A path that leaves the working folder is left out of the
// locations, so the client never shows it as somewhere the agent worked.
func (r *runner) startUpdate(
	id acp.ToolCallId, t toolCall, status acp.ToolCallStatus,
) acp.SessionUpdate {
	kind, _ := toolKindOf(t.Kind)
	opts := []acp.ToolCallStartOpt{
		acp.WithStartKind(kind),
		acp.WithStartStatus(status),
		acp.WithStartRawInput(rawInput(t)),
	}
	if full, err := r.resolve(shownPath(t)); err == nil {
		opts = append(opts, acp.WithStartLocations([]acp.ToolCallLocation{{Path: full}}))
	}
	return acp.StartToolCall(id, t.Title, opts...)
}

// shownPath is the path a tool call is about: its own path, or else the file it writes.
func shownPath(t toolCall) string {
	if t.Path == "" && t.Write != nil {
		return t.Write.Path
	}
	return t.Path
}

// rawInput is the input a client shows for a tool call.
func rawInput(t toolCall) map[string]any {
	in := map[string]any{}
	if p := shownPath(t); p != "" {
		in["path"] = p
	}
	if t.Command != "" {
		in["command"] = t.Command
	}
	return in
}

// finish performs the tool call and reports the result.
func (r *runner) finish(ctx context.Context, id acp.ToolCallId, t toolCall) error {
	return r.update(ctx, resultUpdate(id, r.perform(t)))
}

// resultUpdate is the update that closes a tool call.
func resultUpdate(id acp.ToolCallId, res toolResult) acp.SessionUpdate {
	content := res.content
	if res.text != "" {
		content = append([]acp.ToolCallContent{acp.ToolContent(acp.TextBlock(res.text))}, content...)
	}
	return acp.UpdateToolCall(id,
		acp.WithUpdateStatus(res.status),
		acp.WithUpdateContent(content),
		acp.WithUpdateRawOutput(map[string]any{"output": res.text}),
	)
}

// perform does what a tool call says: it checks its paths and writes its file. A path that leaves
// the working folder fails the call and writes nothing.
func (r *runner) perform(t toolCall) toolResult {
	status, _ := toolStatusOf(t.Status)
	res := toolResult{status: status, text: t.Result}
	if t.Path != "" {
		if _, err := r.resolve(t.Path); err != nil {
			return failedResult(err)
		}
	}
	if t.Write == nil || status != acp.ToolCallStatusCompleted {
		return res
	}
	diff, err := r.writeFile(*t.Write)
	if err != nil {
		return failedResult(err)
	}
	res.content = []acp.ToolCallContent{diff}
	return res
}

// failedResult reports an error as a failed tool call.
func failedResult(err error) toolResult {
	return toolResult{status: acp.ToolCallStatusFailed, text: err.Error()}
}

// resolve turns a scenario path into a path under the working folder. The check is on the text
// of the path only, which is enough for paths that a scenario file spells out.
func (r *runner) resolve(p string) (string, error) {
	if !filepath.IsLocal(p) {
		return "", fmt.Errorf("path %q is outside the working folder", p)
	}
	return filepath.Join(r.cwd, p), nil
}

// writeFile really writes the file and describes the change as a diff.
func (r *runner) writeFile(w fileWrite) (acp.ToolCallContent, error) {
	full, err := r.resolve(w.Path)
	if err != nil {
		return acp.ToolCallContent{}, err
	}
	before, err := os.ReadFile(full)
	existed := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return acp.ToolCallContent{}, fmt.Errorf("read %s: %w", w.Path, err)
	}
	if err := os.MkdirAll(filepath.Dir(full), dirMode); err != nil {
		return acp.ToolCallContent{}, fmt.Errorf("create folder for %s: %w", w.Path, err)
	}
	content := w.text()
	if err := os.WriteFile(full, []byte(content), fileMode); err != nil {
		return acp.ToolCallContent{}, fmt.Errorf("write %s: %w", w.Path, err)
	}
	if existed {
		return acp.ToolDiffContent(full, content, string(before)), nil
	}
	return acp.ToolDiffContent(full, content), nil
}
