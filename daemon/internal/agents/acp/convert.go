package acp

import (
	"strings"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// convertUpdate turns a session update into an event. It returns false for the updates that the
// daemon does not use: echoes of the user's own message, command lists, mode and usage changes.
func convertUpdate(u sdk.SessionUpdate) (agents.AgentEvent, bool) {
	switch {
	case u.AgentMessageChunk != nil:
		text, ok := chunkText(u.AgentMessageChunk.Content)
		return agents.MessageChunk{Text: text}, ok
	case u.AgentThoughtChunk != nil:
		text, ok := chunkText(u.AgentThoughtChunk.Content)
		return agents.ThoughtChunk{Text: text}, ok
	case u.ToolCall != nil:
		return toolCallEvent(u.ToolCall), true
	case u.ToolCallUpdate != nil:
		return toolCallUpdateEvent(u.ToolCallUpdate), true
	case u.Plan != nil:
		return planEvent(u.Plan.Entries), true
	case u.PlanUpdate != nil && u.PlanUpdate.Plan.Items != nil:
		return planEvent(u.PlanUpdate.Plan.Items.Entries), true
	}
	return nil, false
}

// chunkText returns the text of a content block. Other kinds of block (images, audio) have no
// place in a chat message yet.
func chunkText(block sdk.ContentBlock) (string, bool) {
	if block.Text == nil || block.Text.Text == "" {
		return "", false
	}
	return block.Text.Text, true
}

// planEvent turns plan entries into a plan update.
func planEvent(entries []sdk.PlanEntry) agents.PlanUpdate {
	steps := make([]agents.PlanStep, 0, len(entries))
	for _, e := range entries {
		steps = append(steps, agents.PlanStep{Text: e.Content, Status: string(e.Status)})
	}
	return agents.PlanUpdate{Steps: steps}
}

// toolCallEvent turns the start of a tool call into an event.
func toolCallEvent(tc *sdk.SessionUpdateToolCall) agents.ToolCall {
	b := budget{left: agents.MaxContentBytes}
	text, diffs := flattenContent(tc.Content, &b)
	status := string(tc.Status)
	if status == "" {
		status = agents.StatusPending
	}
	return agents.ToolCall{
		ID:        string(tc.ToolCallId),
		Title:     tc.Title,
		Kind:      string(tc.Kind),
		Status:    status,
		Path:      toolPath(tc.Locations, tc.RawInput),
		Command:   rawField(tc.RawInput, "command"),
		Content:   text,
		Diffs:     diffs,
		Truncated: b.cut,
	}
}

// toolCallUpdateEvent turns a progress report on a tool call into an event.
func toolCallUpdateEvent(tc *sdk.SessionToolCallUpdate) agents.ToolCallUpdate {
	b := budget{left: agents.MaxContentBytes}
	text, diffs := flattenContent(tc.Content, &b)
	ev := agents.ToolCallUpdate{ID: string(tc.ToolCallId), Content: text, Diffs: diffs, Truncated: b.cut}
	if tc.Title != nil {
		ev.Title = *tc.Title
	}
	if tc.Status != nil {
		ev.Status = string(*tc.Status)
	}
	return ev
}

// budget shares the MaxContentBytes limit between the pieces of one event.
type budget struct {
	left int
	cut  bool
}

// take returns as much of text as still fits.
func (b *budget) take(text string) string {
	kept, cut := agents.Truncate(text, b.left)
	b.left -= len(kept)
	if cut {
		b.cut = true
	}
	return kept
}

// flattenContent turns the content of a tool call into text and file diffs, within the budget.
func flattenContent(content []sdk.ToolCallContent, b *budget) (string, []agents.FileDiff) {
	var texts []string
	var diffs []agents.FileDiff
	for _, c := range content {
		switch {
		case c.Content != nil && c.Content.Content.Text != nil:
			texts = append(texts, b.take(c.Content.Content.Text.Text))
		case c.Diff != nil:
			diff := agents.FileDiff{Path: c.Diff.Path, NewText: b.take(c.Diff.NewText)}
			if c.Diff.OldText != nil {
				diff.OldText = b.take(*c.Diff.OldText)
			}
			diffs = append(diffs, diff)
		}
	}
	return strings.Join(texts, "\n"), diffs
}

// toolPath finds the file that a tool call is about: its first location, or else a path in its
// input.
func toolPath(locations []sdk.ToolCallLocation, input any) string {
	if len(locations) > 0 && locations[0].Path != "" {
		return locations[0].Path
	}
	return rawField(input, "path", "file_path", "filePath")
}

// rawField reads a text value from a tool call's input, under the first of the keys that has one.
// The input is whatever the agent sent, so anything that is not text is ignored.
func rawField(input any, keys ...string) string {
	fields, ok := input.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range keys {
		if value, ok := fields[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}
