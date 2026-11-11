package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

const (
	toolKindOther   = "other"
	decisionDecline = "decline"
)

func (s *session) dispatch(m rpcMessage) {
	switch m.Method {
	case "turn/started":
		var p struct {
			Thread string `json:"threadId"`
			Turn   struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(m.Params, &p) == nil && p.Thread == s.threadID() {
			s.mu.Lock()
			if s.busy {
				s.setTurn(p.Turn.ID)
			}
			s.mu.Unlock()
		}
	case "thread/tokenUsage/updated":
		s.updateContext(m.Params)
	case "turn/completed":
		var p struct {
			Thread string `json:"threadId"`
			Turn   struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Error  any    `json:"error"`
			} `json:"turn"`
		}
		if json.Unmarshal(m.Params, &p) == nil && s.owns(p.Thread, p.Turn.ID) {
			reason := agents.TurnEndTurn
			detail := ""
			switch p.Turn.Status {
			case "interrupted":
				reason = agents.TurnCancelled
			case "failed":
				reason = agents.TurnError
				detail = fmt.Sprint(p.Turn.Error)
				if detail == "<nil>" {
					detail = "Codex turn failed"
				}
			}
			s.endTurn(reason, detail)
		}
	case "item/agentMessage/delta", "item/reasoning/summaryTextDelta", "item/reasoning/textDelta":
		var p struct {
			Delta  string `json:"delta"`
			Thread string `json:"threadId"`
			Turn   string `json:"turnId"`
		}
		_ = json.Unmarshal(m.Params, &p)
		if s.owns(p.Thread, p.Turn) {
			if strings.Contains(m.Method, "reasoning") {
				s.sink.Emit(agents.ThoughtChunk{Text: p.Delta})
			} else {
				s.sink.Emit(agents.MessageChunk{Text: p.Delta})
			}
		}
	case "turn/plan/updated":
		s.plan(m.Params)
	case "item/started", "item/completed":
		s.item(m.Method, m.Params)
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		s.approval(m)
	case "item/permissions/requestApproval":
		_ = s.write(map[string]any{jsonRPCField: jsonRPCVersion, "id": m.ID, "result": map[string]any{"scope": "turn", "permissions": map[string]any{}}})
	case "serverRequest/resolved":
		s.resolved(m.Params)
	default:
		if m.ID != nil {
			_ = s.write(map[string]any{jsonRPCField: jsonRPCVersion, "id": m.ID, "error": map[string]any{"code": -32601, "message": "unsupported Codex app-server request"}})
		}
	}
}
func (s *session) updateContext(raw json.RawMessage) {
	var update struct {
		Thread string `json:"threadId"`
		Turn   string `json:"turnId"`
		Usage  struct {
			Last struct {
				Tokens int64 `json:"totalTokens"`
			} `json:"last"`
			Window int64 `json:"modelContextWindow"`
		} `json:"tokenUsage"`
	}
	if json.Unmarshal(raw, &update) != nil || update.Thread != s.threadID() || update.Turn == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy && s.turn == update.Turn {
		s.contextUsed = agents.ContextPercent(update.Usage.Last.Tokens, update.Usage.Window)
	}
}

func (s *session) owns(thread, turn string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return thread != "" && turn != "" && thread == s.threadID() && turn != "" && turn == s.turn && s.busy
}
func (s *session) plan(raw json.RawMessage) {
	var p struct {
		Thread string `json:"threadId"`
		Turn   string `json:"turnId"`
		Plan   []struct {
			Step   string `json:"step"`
			Status string `json:"status"`
		} `json:"plan"`
	}
	if json.Unmarshal(raw, &p) != nil || !s.owns(p.Thread, p.Turn) {
		return
	}
	out := agents.PlanUpdate{}
	for _, v := range p.Plan {
		st := v.Status
		if st != "in_progress" && st != "completed" {
			st = "pending"
		}
		out.Steps = append(out.Steps, agents.PlanStep{Text: v.Step, Status: st})
	}
	s.sink.Emit(out)
}
func (s *session) item(method string, raw json.RawMessage) {
	var p struct {
		Thread string         `json:"threadId"`
		Turn   string         `json:"turnId"`
		Item   map[string]any `json:"item"`
	}
	if json.Unmarshal(raw, &p) != nil || !s.owns(p.Thread, p.Turn) {
		return
	}
	id := str(p.Item["id"])
	typ := str(p.Item["type"])
	status := agents.StatusInProgress
	if method == "item/completed" {
		status = mapItemStatus(str(p.Item["status"]))
	}
	if typ == "userMessage" || typ == "agentMessage" || strings.Contains(typ, "reasoning") {
		return
	}
	info := parseItem(p.Item, typ)
	s.mu.Lock()
	if method == "item/started" {
		s.items[id] = info
	} else if old, ok := s.items[id]; ok {
		info = mergeItem(old, info)
		delete(s.items, id)
	}
	s.mu.Unlock()
	if method == "item/started" {
		s.sink.Emit(agents.ToolCall{ID: id, Title: info.title, Kind: info.kind, Status: status, Path: info.path, Command: info.command, Content: info.description, Diffs: info.changes})
	} else {
		s.sink.Emit(agents.ToolCallUpdate{ID: id, Title: info.title, Status: status, Content: info.description, Diffs: info.changes})
	}
}
func mapItemStatus(v string) string {
	switch v {
	case "failed", "declined", "cancelled":
		return agents.StatusFailed
	default:
		return agents.StatusCompleted
	}
}
func parseItem(item map[string]any, typ string) itemInfo {
	description := str(item["stdout"])
	if description == "" {
		description = str(item["aggregatedOutput"])
	}
	v := itemInfo{title: typ, kind: toolKindOther, command: str(item["command"]), description: description}
	if typ == "mcpToolCall" {
		v.title, v.description = mcpDetails(item)
	}
	if t := str(item["title"]); t != "" {
		v.title = t
	}
	if typ == "commandExecution" {
		v.kind = "execute"
	}
	if typ == "fileChange" {
		v.kind = "edit"
		v.changes = parseDiffs(item["changes"])
		if len(v.changes) == 1 {
			v.path = v.changes[0].Path
		} else if len(v.changes) > 1 {
			v.kind = toolKindOther
			v.description = fmt.Sprintf("Changes to %d files", len(v.changes))
		}
	}
	return v
}
func parseDiffs(value any) []agents.FileDiff {
	items, _ := value.([]any)
	out := make([]agents.FileDiff, 0, len(items))
	for _, raw := range items {
		m, _ := raw.(map[string]any)
		out = append(out, agents.FileDiff{Path: str(m["path"]), NewText: str(m["diff"])})
	}
	return out
}
func mergeItem(a, b itemInfo) itemInfo {
	if b.title != "" {
		a.title = b.title
	}
	if b.command != "" {
		a.command = b.command
	}
	if b.description != "" {
		a.description = b.description
	}
	if len(b.changes) > 0 {
		a.changes = b.changes
	}
	if b.path != "" {
		a.path = b.path
	}
	return a
}
func str(v any) string { x, _ := v.(string); return x }
func (s *session) approval(m rpcMessage) {
	var p struct {
		Thread    string                           `json:"threadId"`
		Turn      string                           `json:"turnId"`
		Item      string                           `json:"itemId"`
		Command   *string                          `json:"command"`
		Reason    *string                          `json:"reason"`
		Available []json.RawMessage                `json:"availableDecisions"`
		Path      *string                          `json:"path"`
		Network   *struct{ Host, Protocol string } `json:"networkApprovalContext"`
	}
	_ = json.Unmarshal(m.Params, &p)
	if !s.owns(p.Thread, p.Turn) {
		s.answerDenied(m.ID, p.Available)
		return
	}
	s.mu.Lock()
	info := s.items[p.Item]
	s.mu.Unlock()
	opts, optionMap := approvalOptions(p.Available)
	if len(opts) == 0 {
		s.answerDenied(m.ID, p.Available)
		return
	}
	req := fmt.Sprintf("%s:%s", m.Method, string(m.ID))
	s.mu.Lock()
	s.pending[req] = pendingApproval{id: m.ID, options: optionMap}
	s.mu.Unlock()
	title := info.title
	if title == "" {
		title = "Review Codex action"
	}
	if p.Reason != nil && *p.Reason != "" {
		title = *p.Reason
	}
	command := info.command
	if p.Command != nil {
		command = *p.Command
	}
	path := info.path
	if p.Path != nil {
		path = *p.Path
	}
	kind := info.kind
	if m.Method == "item/commandExecution/requestApproval" {
		kind = "execute"
	}
	if m.Method == "item/fileChange/requestApproval" && len(info.changes) == 0 {
		if path == "" {
			s.mu.Lock()
			delete(s.pending, req)
			s.mu.Unlock()
			s.answerDenied(m.ID, p.Available)
			return
		}
		kind = "edit"
		title = "Review file changes"
	}
	paths := make([]string, 0, len(info.changes))
	for _, change := range info.changes {
		paths = append(paths, change.Path)
	}
	if len(info.changes) > 1 {
		kind = toolKindOther
		path = ""
		title = fmt.Sprintf("%s (%d file changes)", title, len(info.changes))
		command = describeChanges(info.changes)
	}
	if p.Network != nil {
		kind = toolKindOther
		title = fmt.Sprintf("Network access to %s (%s)", p.Network.Host, p.Network.Protocol)
	}
	s.sink.Emit(agents.PermissionRequested{RequestID: req, ToolCallID: p.Item, Title: title, Kind: kind, Path: path, Paths: paths, Command: command, Options: opts})
}

func describeChanges(changes []agents.FileDiff) string {
	parts := make([]string, 0, len(changes))
	for _, change := range changes {
		parts = append(parts, fmt.Sprintf("%s\n%s", change.Path, change.NewText))
	}
	return strings.Join(parts, "\n\n")
}
func approvalOptions(raw []json.RawMessage) ([]agents.PermissionOption, map[string]string) {
	allowed := map[string]bool{"accept": true, decisionDecline: true}
	if raw != nil {
		allowed = map[string]bool{}
		for _, b := range raw {
			var v string
			if json.Unmarshal(b, &v) == nil {
				allowed[v] = true
			}
		}
	}
	if !allowed[decisionDecline] && !allowed["cancel"] {
		return nil, nil
	}
	out := []agents.PermissionOption{}
	opts := map[string]string{}
	if allowed["accept"] {
		out = append(out, agents.PermissionOption{ID: "accept", Name: "Allow once", Kind: agents.OptionAllowOnce})
		opts["accept"] = "accept"
	}
	deny := decisionDecline
	if !allowed[deny] {
		deny = "cancel"
	}
	out = append(out, agents.PermissionOption{ID: deny, Name: "Deny", Kind: agents.OptionRejectOnce})
	opts[deny] = deny
	return out, opts
}
func (s *session) answerDenied(id json.RawMessage, available []json.RawMessage) {
	_, choices := approvalOptions(available)
	decision := decisionDecline
	if _, ok := choices[decision]; !ok {
		if _, ok = choices["cancel"]; ok {
			decision = "cancel"
		} else {
			_ = s.write(map[string]any{jsonRPCField: jsonRPCVersion, "id": id, "error": map[string]any{"code": -32602, "message": "no safe approval denial is available"}})
			return
		}
	}
	_ = s.write(map[string]any{jsonRPCField: jsonRPCVersion, "id": id, "result": map[string]any{"decision": decision}})
}
func (s *session) resolved(raw json.RawMessage) {
	var p struct {
		ID        json.RawMessage `json:"id"`
		RequestID json.RawMessage `json:"requestId"`
		Thread    string          `json:"threadId"`
	}
	_ = json.Unmarshal(raw, &p)
	id := p.RequestID
	if id == nil {
		id = p.ID
	}
	s.mu.Lock()
	var cancelled []string
	for key, v := range s.pending {
		if string(v.id) == string(id) && (p.Thread == "" || p.Thread == s.threadID()) {
			delete(s.pending, key)
			cancelled = append(cancelled, key)
		}
	}
	s.mu.Unlock()
	for _, requestID := range cancelled {
		s.sink.Emit(agents.PermissionCancelled{RequestID: requestID})
	}
}
func (s *session) respondApproval(ctx context.Context, r agents.ApprovalResponse) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	p, ok := s.pending[r.RequestID]
	if !ok {
		s.mu.Unlock()
		return agents.ErrUnknownRequest
	}
	decision := decisionDecline
	if r.Cancelled {
		if _, ok = p.options["cancel"]; ok {
			decision = "cancel"
		}
	} else {
		var exists bool
		decision, exists = p.options[r.OptionID]
		if !exists {
			s.mu.Unlock()
			return agents.ErrUnknownOption
		}
	}
	delete(s.pending, r.RequestID)
	s.mu.Unlock()
	return s.write(map[string]any{jsonRPCField: jsonRPCVersion, "id": p.id, "result": map[string]any{"decision": decision}})
}
