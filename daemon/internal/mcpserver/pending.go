package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The tools whose part of Marshal is not built yet: the five that speak for checklists, comments, and
// attachments (docs/architecture.md section 19.1 and 19.2, which arrive with the advanced card work),
// and search_codebase's dependence on the codebase map (build-plan task 7.9).
//
// They are registered anyway, and each answers with a sentence. That is a deliberate choice over the
// two alternatives, and both alternatives are worse:
//
//   - Leaving them out of the tool list. An agent that has read the card's comments elsewhere, or
//     that knows the convention, will try to call one; a client that is not offered a tool it calls
//     gets a protocol error about an unknown tool, which tells the agent nothing about Marshal and
//     invites it to try the same thing another way.
//   - Answering them with emptiness - "no checklists". That is a lie an agent will act on: it will
//     conclude the card has no checklist rather than that Marshal cannot say, and it will carry on
//     believing it has read the whole of what the owner wrote.
//
// So each one says what is missing, says plainly that this is not a permission refusal (so the agent
// does not go looking for a mode to change), and says what to do instead. The handlers keep their
// real arguments and their real answer shapes, the gate runs first like everywhere else, and the day
// the domain lands the refusal is replaced by the work - the tool, its name, and its arguments do not
// change under the agents that already know them.
const (
	// noChecklists is list_checklists' answer.
	noChecklists = "Marshal does not keep checklists yet, so there is nothing to read. " +
		"This is not a permission refusal: carry on with the work, and if an item should be ticked, say so in your answer instead."

	// noTick is tick_checklist_item's answer.
	noTick = "Marshal does not keep checklists yet, so nothing was ticked. " +
		"This is not a permission refusal: carry on, and say in your answer what you finished and how you know it is done."

	// noComments is read_comments' answer.
	noComments = "Marshal does not keep comments yet, so there is nothing to read. " +
		"This is not a permission refusal: carry on with the work, and take the card's description as the whole of what has been said to you."

	// noCommentPosted is post_comment's answer.
	noCommentPosted = "Marshal does not keep comments yet, so nothing was posted. " +
		"This is not a permission refusal: say what you wanted to say in your answer, and the card's owner will see it there."

	// noAttachment is read_attachment's answer.
	noAttachment = "Marshal does not hold attachments yet, so there is nothing to read. " +
		"This is not a permission refusal: carry on with what the card's description says, and ask the card's owner if a file is missing."

	// noCodebase is search_codebase's answer while the codebase map is unbuilt.
	noCodebase = "Marshal has not built the codebase map yet, so there is nothing to search. " +
		"This is not a permission refusal: find the file by reading the tree, or by searching it with the tools you already have."
)

// listChecklistsInput is list_checklists' arguments: there are none. The card is the one the server
// is served to.
type listChecklistsInput struct{}

// checklistItemOut is one item on one of a card's checklists, in the shape the tool will answer with.
// It is declared now so that the arguments and the answer do not change when the domain lands.
type checklistItemOut struct {
	// ID is the item's id, which tick_checklist_item takes.
	ID string `json:"id"`
	// Text is what the item says.
	Text string `json:"text"`
	// Done is whether it is ticked.
	Done bool `json:"done"`
	// DoneBy is who ticked it: "person", "agent", or "daemon" (an item whose evidence unticks
	// itself is the daemon's doing).
	DoneBy string `json:"doneBy,omitempty"`
	// Evidence is what makes the item true: a commit, a file, a test. It is what the daemon watches
	// to untick an item whose evidence stops being true.
	Evidence string `json:"evidence,omitempty"`
}

// checklistOut is one of a card's checklists with its items.
type checklistOut struct {
	// ID is the checklist's id.
	ID string `json:"id"`
	// Title is what the checklist is called.
	Title string `json:"title"`
	// PersonOnly is true for a checklist only a person may tick. tick_checklist_item refuses on one
	// of these, whatever the mode.
	PersonOnly bool `json:"personOnly,omitempty"`
	// Items are the checklist's items, in order. Never null.
	Items []checklistItemOut `json:"items"`
}

// listChecklistsOut is list_checklists' answer.
type listChecklistsOut struct {
	// Checklists are the card's checklists. Never null.
	Checklists []checklistOut `json:"checklists"`
}

// list_checklists reads this card's checklists.
func (s *Server) listChecklists(ctx context.Context, _ *mcp.CallToolRequest, _ listChecklistsInput) (*mcp.CallToolResult, listChecklistsOut, error) {
	if err := s.allow("list_checklists", kindRead); err != nil {
		return nil, listChecklistsOut{}, err
	}
	if s.deps.Panel == nil {
		return nil, listChecklistsOut{Checklists: []checklistOut{}}, notBuilt(noChecklists)
	}
	lists, err := s.deps.Panel.Checklists(ctx, s.identity.CardID)
	if err != nil {
		return nil, listChecklistsOut{}, fmt.Errorf("read the checklists: %w", err)
	}
	out := listChecklistsOut{Checklists: make([]checklistOut, 0, len(lists.Checklists))}
	for _, l := range lists.Checklists {
		out.Checklists = append(out.Checklists, checklistOf(l))
	}
	return nil, out, nil
}

// tickChecklistItemInput is tick_checklist_item's arguments.
type tickChecklistItemInput struct {
	// ItemID is the item to tick or untick, as list_checklists gives it.
	ItemID string `json:"itemId"`
	// Done is the state to set: true ticks, false unticks.
	Done bool `json:"done"`
	// Evidence is what makes the item true, such as the commit or the test that proves it. A tick
	// with no evidence is refused once this is built.
	Evidence string `json:"evidence,omitempty"`
}

// tickChecklistItemOut is tick_checklist_item's answer.
type tickChecklistItemOut struct {
	// Item is the item as it now stands.
	Item checklistItemOut `json:"item"`
}

// tick_checklist_item ticks or unticks one item on this card's checklist.
func (s *Server) tickChecklistItem(ctx context.Context, _ *mcp.CallToolRequest, in tickChecklistItemInput) (*mcp.CallToolResult, tickChecklistItemOut, error) {
	if err := s.allow("tick_checklist_item", kindWrite); err != nil {
		return nil, tickChecklistItemOut{}, err
	}
	if s.deps.Panel == nil {
		return nil, tickChecklistItemOut{}, notBuilt(noTick)
	}
	lists, err := s.deps.Panel.AgentTick(ctx, s.identity.CardID, in.ItemID, in.Done, in.Evidence)
	if err != nil {
		return nil, tickChecklistItemOut{}, err
	}
	for _, l := range lists.Checklists {
		for _, it := range checklistOf(l).Items {
			if it.ID == in.ItemID {
				return nil, tickChecklistItemOut{Item: it}, nil
			}
		}
	}
	return nil, tickChecklistItemOut{}, errors.New("that checklist item is gone")
}

// readCommentsInput is read_comments' arguments: there are none.
type readCommentsInput struct{}

// commentOut is one comment on a card.
type commentOut struct {
	// ID is the comment's id.
	ID string `json:"id"`
	// Author is who wrote it: "person" or "agent".
	Author string `json:"author"`
	// Body is what it says, in markdown.
	Body string `json:"body"`
	// CreatedAt is when it was posted, in RFC 3339.
	CreatedAt string `json:"createdAt"`
}

// readCommentsOut is read_comments' answer.
type readCommentsOut struct {
	// Comments are the comments this agent has not been shown, oldest first. Reading them is what
	// marks them as read, which is what the card's owner sees as "agent read this". Never null.
	Comments []commentOut `json:"comments"`
}

// read_comments reads this card's new comments and marks them read.
func (s *Server) readComments(ctx context.Context, _ *mcp.CallToolRequest, _ readCommentsInput) (*mcp.CallToolResult, readCommentsOut, error) {
	if err := s.allow("read_comments", kindRead); err != nil {
		return nil, readCommentsOut{}, err
	}
	if s.deps.Panel == nil {
		return nil, readCommentsOut{Comments: []commentOut{}}, notBuilt(noComments)
	}
	unread, err := s.deps.Panel.UnreadComments(ctx, s.identity.CardID)
	if err != nil {
		return nil, readCommentsOut{}, fmt.Errorf("read the comments: %w", err)
	}
	out := readCommentsOut{Comments: make([]commentOut, 0, len(unread))}
	for _, c := range unread {
		out.Comments = append(out.Comments, commentOf(c))
	}
	return nil, out, nil
}

// postCommentInput is post_comment's arguments.
type postCommentInput struct {
	// Body is the comment, in markdown.
	Body string `json:"body"`
}

// post_comment posts a comment on this card as its agent.
func (s *Server) postComment(ctx context.Context, _ *mcp.CallToolRequest, in postCommentInput) (*mcp.CallToolResult, commentOut, error) {
	if err := s.allow("post_comment", kindWrite); err != nil {
		return nil, commentOut{}, err
	}
	if s.deps.Panel == nil {
		return nil, commentOut{}, notBuilt(noCommentPosted)
	}
	posted, err := s.deps.Panel.PostAsAgent(ctx, s.identity.CardID, in.Body)
	if err != nil {
		return nil, commentOut{}, err
	}
	return nil, commentOf(posted), nil
}

// readAttachmentInput is read_attachment's arguments.
type readAttachmentInput struct {
	// Path is the attachment's name as the card shows it.
	Path string `json:"path"`
}

// readAttachmentOut is read_attachment's answer. The bytes arrive as text; an attachment that is not
// text is refused once this is built, because an agent's context is text.
type readAttachmentOut struct {
	// Path is the attachment that was read.
	Path string `json:"path"`
	// Content is its text.
	Content string `json:"content"`
}

// read_attachment reads a file attached to this card.
func (s *Server) readAttachment(ctx context.Context, _ *mcp.CallToolRequest, in readAttachmentInput) (*mcp.CallToolResult, readAttachmentOut, error) {
	if err := s.allow("read_attachment", kindRead); err != nil {
		return nil, readAttachmentOut{}, err
	}
	if s.deps.Panel == nil {
		return nil, readAttachmentOut{}, notBuilt(noAttachment)
	}
	text, err := s.deps.Panel.ReadAttachment(ctx, s.identity.CardID, in.Path)
	if err != nil {
		return nil, readAttachmentOut{}, err
	}
	return nil, readAttachmentOut{Path: in.Path, Content: text}, nil
}

// notBuilt is the answer a tool gives while the part of Marshal it speaks for does not exist. It is a
// plain error, so the SDK hands it back as a tool error the model reads.
func notBuilt(sentence string) error { return errors.New(sentence) }

// checklistOf maps one wire checklist to the tool's answer.
func checklistOf(l protocol.Checklist) checklistOut {
	out := checklistOut{ID: l.ID, Title: l.Name, PersonOnly: l.PeopleOnly, Items: make([]checklistItemOut, 0, len(l.Items))}
	for _, it := range l.Items {
		out.Items = append(out.Items, checklistItemOut{ID: it.ID, Text: it.Text, Done: it.Done, DoneBy: it.DoneByKind})
	}
	return out
}

// commentOf maps one wire comment to the tool's answer.
func commentOf(c protocol.Comment) commentOut {
	return commentOut{
		ID: c.ID, Author: string(c.AuthorKind), Body: c.Body,
		CreatedAt: c.CreatedAt.Time().UTC().Format(time.RFC3339),
	}
}
