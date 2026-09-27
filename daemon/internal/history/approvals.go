package history

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The approval flow's own half of history (S8b, docs/backend-checklist.md B3.4, N7). RecordsOf
// cannot write an approval's row: the row must carry the approval's own id, and nothing has minted
// that id at the moment an agent's PermissionRequested event is logged (see RecordsOf's own
// comment). So the approvals work writes its row explicitly, through AppendApproval, once the id
// exists - and, because that row is a person's answer waiting to happen rather than a fact that
// already happened, it is also the one kind of history row this package ever rewrites in place:
// ResolveApproval updates the state a stored request stands at, found by the approval's own id, so
// a chat reopened after the request was answered shows the right thing instead of a stuck "waiting"
// block with dead buttons.

// approvalEventPageSize is how many of an owner's approval events one page read asks for, while
// ResolveApproval looks for the one that carries the approval's id. It is generous: a card or a
// chat rarely takes more than a handful of approvals over its whole life, so almost every search
// finds it on the first page, and the loop only pages further for one that has taken very many.
const approvalEventPageSize = 100

// AppendApproval stores one permission request as a card's next history event, with the approval's
// own id inside its detail (S8b): the id a client answers it by (POST /v1/approvals/{id}) is on
// the row from the moment it is written, so a chat reopened before anyone answers can still call
// it. State lets the same call serve two moments the approvals work has: StateWaiting for a
// request a person is being asked, and an already-decided state for one the daemon answered on its
// own (bypass, the harness auto-answer path) - which is why approvalID may be empty here: those two
// paths never mint an approval id, or a row in the approvals table, at all.
func (s *Store) AppendApproval(ctx context.Context, cardID, sessionID, approvalID string, state State, e agents.PermissionRequested) error {
	return s.appendApproval(ctx, cardOwner(cardID), sessionID, approvalID, state, e)
}

// AppendChatApproval is AppendApproval for a chat's session.
func (s *Store) AppendChatApproval(ctx context.Context, chatID, sessionID, approvalID string, state State, e agents.PermissionRequested) error {
	return s.appendApproval(ctx, chatOwner(chatID), sessionID, approvalID, state, e)
}

// appendApproval is the shared tail of AppendApproval and AppendChatApproval.
func (s *Store) appendApproval(ctx context.Context, o owner, sessionID, approvalID string, state State, e agents.PermissionRequested) error {
	detail, err := encodeDetail(approvalDetail{
		ID: approvalID, RequestID: e.RequestID, Title: e.Title, RequestKind: e.Kind,
		Path: e.Path, Command: e.Command,
	})
	if err != nil {
		return fmt.Errorf("store an approval of %s: %w", o.label(), err)
	}
	_, err = s.append(ctx, o, sessionID, []Record{
		{Kind: KindApproval, State: state, Summary: e.Title, Detail: detail},
	})
	return err
}

// ResolveApproval rewrites a card's stored approval row to the state it was answered with, once
// Manager.Respond (or the daemon withdrawing a request whose session went away) decides it. The row
// is found by the approval's own id rather than by the row's own id: the caller of this method only
// ever has the approval's id (it is what POST /v1/approvals/{id} takes), never the history event id
// AppendApproval happened to write it under.
//
// A card with no matching row is left alone rather than treated as an error: the row may belong to
// a request the daemon answered on its own (AppendApproval with an empty approvalID, which this
// call can never find, by design), or it may simply not exist for this card at all if the caller
// passed the wrong owner. Either way, every live view has already been told the right state through
// approval.resolved; this call only keeps a chat that is reopened later in step.
func (s *Store) ResolveApproval(ctx context.Context, cardID, approvalID string, state State) error {
	return s.resolveApproval(ctx, cardOwner(cardID), approvalID, state)
}

// ResolveChatApproval is ResolveApproval for a chat's session.
func (s *Store) ResolveChatApproval(ctx context.Context, chatID, approvalID string, state State) error {
	return s.resolveApproval(ctx, chatOwner(chatID), approvalID, state)
}

// resolveApproval is the shared tail of ResolveApproval and ResolveChatApproval. It runs the find
// and the rewrite in one write transaction, so a concurrent append of the same owner's history
// cannot land between them.
func (s *Store) resolveApproval(ctx context.Context, o owner, approvalID string, state State) error {
	if approvalID == "" {
		// AppendApproval never gave this row an id (a request the daemon answered on its own), so
		// there is nothing this call could ever find. Asking is not a mistake; finding nothing would
		// look the same, so this is answered the same way without a wasted read.
		return nil
	}
	return s.store.Write(ctx, func(q *db.Queries) error {
		id, found, err := findApprovalEvent(ctx, q, o, approvalID)
		if err != nil {
			return fmt.Errorf("find the stored approval %s of %s: %w", approvalID, o.label(), err)
		}
		if !found {
			return nil
		}
		if o.chat {
			_, err := q.UpdateChatEventState(ctx, db.UpdateChatEventStateParams{
				State: string(state), ID: id, ChatID: &o.id,
			})
			return err
		}
		_, err = q.UpdateSessionEventState(ctx, db.UpdateSessionEventStateParams{
			State: string(state), ID: id, CardID: o.id,
		})
		return err
	})
}

// findApprovalEvent pages an owner's approval events, newest first, until it finds the one whose
// stored detail carries this approval's id, and returns that event's own id (the row AppendApproval
// wrote, which is what the rewrite below addresses by - never the approval's id, which is not a
// column). found is false when no page holds it, which is not an error to the caller: see
// resolveApproval's own doc comment on why that happens and why it is fine.
func findApprovalEvent(ctx context.Context, q *db.Queries, o owner, approvalID string) (id string, found bool, err error) {
	cursor := int64(0)
	for {
		rows, err := approvalEventPage(ctx, q, o, cursor)
		if err != nil {
			return "", false, err
		}
		for _, row := range rows {
			var detail approvalDetail
			if row.DetailJSON == "" {
				continue
			}
			if err := json.Unmarshal([]byte(row.DetailJSON), &detail); err != nil {
				// A row this call cannot read is not this search's to fail on: it is either not an
				// approval this package wrote the way it writes them today, or it is unrelated.
				continue
			}
			if detail.ID == approvalID {
				return row.ID, true, nil
			}
		}
		if len(rows) < approvalEventPageSize {
			return "", false, nil
		}
		cursor = rows[len(rows)-1].Seq
	}
}

// approvalEventPage reads one page of an owner's approval events, oldest-first within the page but
// newest page first overall (the same "before this cursor" order every other page in this package
// reads), using whichever of the two kind-filtered list queries fits the owner.
func approvalEventPage(ctx context.Context, q *db.Queries, o owner, cursor int64) ([]db.SessionEvent, error) {
	limit := int64(approvalEventPageSize)
	if o.chat {
		return q.ListChatEventsByKind(ctx, db.ListChatEventsByKindParams{
			ChatID: &o.id, Kind: string(KindApproval), Seq: before(cursor), Limit: limit,
		})
	}
	return q.ListSessionEventsByKind(ctx, db.ListSessionEventsByKindParams{
		CardID: o.id, Kind: string(KindApproval), Seq: before(cursor), Limit: limit,
	})
}
