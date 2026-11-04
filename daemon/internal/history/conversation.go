package history

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/providers"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Account for row metadata too, so millions of empty events cannot evade the bound.
const conversationRowOverhead = 256

// Conversation reads the saved card or chat transcript for a native built-in ID.
// Native tool inputs are not in the display history, so recorded tool activity is
// supplied as text context. It must not be invented as a new executable tool call.
func (s *Store) Conversation(ctx context.Context, agentID string) ([]providers.Message, error) {
	if agentID == "" {
		return nil, errors.New("a built-in session ID is required")
	}
	var rows []db.SessionEvent
	err := s.store.Read(ctx, func(q *db.Queries) error {
		ids, err := q.FindBuiltinConversationSession(ctx, agentID)
		if err != nil {
			return err
		}
		if len(ids) != 1 {
			return errors.New("the saved built-in session could not be identified uniquely")
		}
		rows, err = s.conversationRows(ctx, q, ids[0])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("read the saved conversation: %w", err)
	}
	slices.Reverse(rows)
	return conversationMessages(rows)
}

func (s *Store) conversationRows(ctx context.Context, q *db.Queries, sessionID string) ([]db.SessionEvent, error) {
	var out []db.SessionEvent
	cursor, bytes := int64(math.MaxInt64), 0
	for {
		page, err := q.ListConversationEvents(ctx, db.ListConversationEventsParams{
			SessionID: sessionID, Seq: cursor, Limit: MaxPageSize,
		})
		if err != nil {
			return nil, err
		}
		for _, row := range page {
			bytes += len(row.Summary) + len(row.DetailJSON) + conversationRowOverhead
			if bytes > s.conversationBytes {
				return nil, errors.New("the saved conversation exceeds the resume size limit")
			}
			out = append(out, row)
		}
		if len(page) < MaxPageSize {
			return out, nil
		}
		cursor = page[len(page)-1].Seq
	}
}

func conversationMessages(rows []db.SessionEvent) ([]providers.Message, error) {
	var messages []providers.Message
	var text strings.Builder
	role := ""
	flush := func() {
		if text.Len() > 0 {
			messages = append(messages, providers.Message{Role: role, Parts: []providers.Part{{Kind: providers.PartText, Text: text.String()}}})
			text.Reset()
		}
	}
	for _, row := range rows {
		nextRole, content := providers.RoleAssistant, row.Summary
		switch Kind(row.Kind) {
		case KindUser:
			nextRole = providers.RoleUser
		case KindToolCall, KindToolCallUpdate:
			var err error
			content, err = conversationTool(row)
			if err != nil {
				return nil, err
			}
		}
		if nextRole != role || nextRole == providers.RoleUser {
			flush()
			role = nextRole
		}
		text.WriteString(content)
	}
	flush()
	return messages, nil
}

func conversationTool(row db.SessionEvent) (string, error) {
	tool, err := storedToolDetail(Event{Kind: Kind(row.Kind), Detail: row.DetailJSON})
	if err != nil {
		return "", err
	}
	var text strings.Builder
	fmt.Fprintf(&text, "\n[Recorded tool activity: %s; state: %s]\nPath: %s\nCommand: %s\n%s\n", firstText(row.Summary, tool.Title), row.State, tool.Path, tool.Command, tool.Content)
	for _, diff := range tool.Diffs {
		fmt.Fprintf(&text, "File: %s\nBefore:\n%s\nAfter:\n%s\n", diff.Path, diff.OldText, diff.NewText)
	}
	if tool.Truncated {
		text.WriteString("[Recorded tool output was truncated; inspect the files again if needed.]\n")
	}
	return text.String(), nil
}
