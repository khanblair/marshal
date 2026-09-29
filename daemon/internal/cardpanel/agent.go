package cardpanel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The panel as the card's own agent uses it, through the internal MCP server (docs/architecture.md
// section 19.1 and 19.2): tick a line with evidence, read the comments it has not seen, post a
// comment, and read a kept file as text.

// maxAgentFileBytes is the most of a kept file an agent is handed, since its context is text.
const maxAgentFileBytes = 256 << 10

type noteEvidence struct {
	Note string `json:"note"`
}

// AgentTick ticks or reopens a line for the card's agent. Ticking needs evidence. Evidence that
// names a check (by its id or its name) that has passed is kept as that check, so the line is
// reopened when the check later fails; any other evidence is kept as the agent's own words. A
// people-only list refuses the agent either way.
func (s *Service) AgentTick(ctx context.Context, cardID, itemID string, done bool, proof string) (protocol.ChecklistList, error) {
	if _, err := s.card(ctx, cardID); err != nil {
		return protocol.ChecklistList{}, err
	}
	var item db.ChecklistItem
	err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		item, err = q.GetChecklistItem(ctx, itemID)
		return err
	})
	if err != nil {
		return protocol.ChecklistList{}, protocol.NotFound("checklist item").With("id", itemID)
	}
	if !done {
		return s.Tick(ctx, cardID, item.ChecklistID, itemID, false, Actor{Agent: true})
	}
	proof = strings.TrimSpace(proof)
	if proof == "" {
		return protocol.ChecklistList{}, protocol.InvalidArgument("Say what proves this line is done before you tick it.")
	}
	if checkID := s.passedCheck(ctx, cardID, proof); checkID != "" {
		return s.TickWithCheck(ctx, cardID, item.ChecklistID, itemID, checkID)
	}
	list, err := s.checklistOf(ctx, cardID, item.ChecklistID)
	if err != nil {
		return protocol.ChecklistList{}, err
	}
	if flag(list.PeopleOnly) {
		return protocol.ChecklistList{}, protocol.Forbidden(messagePeopleOnly)
	}
	body, _ := json.Marshal(noteEvidence{Note: proof})
	return s.writeTick(ctx, cardID, itemID, true, Actor{Agent: true}, string(body))
}

// passedCheck finds the card's passed check that the evidence names, or answers empty.
func (s *Service) passedCheck(ctx context.Context, cardID, proof string) string {
	var rows []db.CardCheck
	if err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		rows, err = q.ListCardChecks(ctx, cardID)
		return err
	}); err != nil {
		return ""
	}
	for _, row := range rows {
		var spec checkSpec
		_ = json.Unmarshal([]byte(row.SpecJSON), &spec)
		if row.Status == string(protocol.CheckStatusPassed) && (row.ID == proof || strings.EqualFold(spec.Name, proof)) {
			return row.ID
		}
	}
	return ""
}

// UnreadComments answers the comments the agent has not read, oldest first, and marks them read.
func (s *Service) UnreadComments(ctx context.Context, cardID string) ([]protocol.Comment, error) {
	if _, err := s.card(ctx, cardID); err != nil {
		return nil, err
	}
	var rows []db.Comment
	if err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		rows, err = q.ListUnreadComments(ctx, cardID)
		return err
	}); err != nil {
		return nil, fmt.Errorf("list the unread comments of card %s: %w", cardID, err)
	}
	out := make([]protocol.Comment, 0, len(rows))
	for _, row := range rows {
		out = append(out, protocol.Comment{
			ID: row.ID, AuthorKind: protocol.AuthorKind(row.AuthorKind), AuthorID: row.AuthorID, Body: row.Body,
			Attachments: []protocol.Attachment{}, CreatedAt: stamp(row.CreatedAt),
		})
	}
	if len(out) == 0 {
		return out, nil
	}
	now := ms(s.now())
	err := s.store.Write(ctx, func(q *db.Queries) error {
		return q.MarkCommentsReadByAgent(ctx, db.MarkCommentsReadByAgentParams{AgentReadAt: &now, CardID: cardID})
	})
	if err != nil {
		return nil, fmt.Errorf("mark the comments of card %s read: %w", cardID, err)
	}
	s.publish(cardID, protocol.EventTypeCommentReadByAgent)
	return out, nil
}

// PostAsAgent posts a comment written by the card's agent and answers it.
func (s *Service) PostAsAgent(ctx context.Context, cardID, body string) (protocol.Comment, error) {
	list, err := s.Post(ctx, cardID, protocol.PostCommentRequest{Body: body, Attachments: []protocol.NewAttachment{}}, Actor{Agent: true})
	if err != nil {
		return protocol.Comment{}, err
	}
	return list.Comments[len(list.Comments)-1], nil
}

// ReadAttachment answers the text of a file kept with one of the card's comments, found by the name
// it was attached with. A file that is not text, or is too big for an agent's context, is refused.
func (s *Service) ReadAttachment(ctx context.Context, cardID, name string) (string, error) {
	if _, err := s.card(ctx, cardID); err != nil {
		return "", err
	}
	var files []db.Attachment
	if err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		files, err = q.ListCardAttachments(ctx, cardID)
		return err
	}); err != nil {
		return "", fmt.Errorf("list the attachments of card %s: %w", cardID, err)
	}
	for _, f := range files {
		if f.MimeType == linkMime || !strings.EqualFold(f.FileName, filepath.Base(name)) {
			continue
		}
		if f.SizeBytes > maxAgentFileBytes {
			return "", protocol.Refused("That file is too big to read as text.")
		}
		data, err := os.ReadFile(filepath.Join(s.files, f.Path))
		if err != nil {
			return "", protocol.NotFound("attachment").With("name", name)
		}
		if !utf8.Valid(data) {
			return "", protocol.Refused("That file is not text, so it cannot be read here.")
		}
		return string(data), nil
	}
	return "", protocol.NotFound("attachment").With("name", name)
}
