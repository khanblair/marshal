package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A card's chat and its activity (docs/backend-checklist.md B2.6 and B2.8), and a project chat's
// messages (B2.10), which are the same stored events under the same wire types. The routes page the
// stored history through cardhistory.Service, which decides the page size and maps every stored
// event; the handlers below only read the address and the query, and write the answer.
//
// The tool detail a chat block opens is served by its own route. The page never carries it: a list
// of a hundred tool calls would otherwise carry a hundred full diffs and outputs.

// historyCursor is where the next page of a card's history starts: the sequence number of the
// last event the page read. It is a cursor like any other, opaque to clients and made by
// EncodeCursor, so its shape can grow a field later without a new API version.
type historyCursor struct {
	Seq int64 `json:"seq"`
}

// listMessages is GET /v1/cards/{id}/messages: the card's chat, newest first, paged with `limit`
// and `cursor`. An unknown card is not found. A card with no history is an empty page, never null.
func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	limit, position, err := readPage(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	page, err := s.history.Messages(r.Context(), id, position, limit)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	answer, err := historyPage(page.Items, page.Cursor, page.More, s.now())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}

// listChatMessages is GET /v1/chats/{id}/messages: a project chat's messages, newest first, paged
// the same way a card's are. An unknown chat is not found. A chat nobody has spoken to is an empty
// page, never null.
func (s *Server) listChatMessages(w http.ResponseWriter, r *http.Request) {
	id, err := chatIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	limit, position, err := readPage(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	page, err := s.history.ChatMessages(r.Context(), id, position, limit)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	answer, err := historyPage(page.Items, page.Cursor, page.More, s.now())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}

// getChatMessage is GET /v1/chats/{id}/messages/{messageId}: one of a chat's messages in full, for
// the tool detail a block opens on demand, as a card's do. A message that is not on this chat, or
// an id that cannot exist, is not found.
func (s *Server) getChatMessage(w http.ResponseWriter, r *http.Request) {
	id, err := chatIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	messageID := r.PathValue("messageId")
	if len(messageID) > maxEchoedIDBytes {
		s.writeError(w, notFoundID("message", messageID))
		return
	}
	detail, err := s.history.ChatMessageDetail(r.Context(), id, messageID)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, detail)
}

// listActivity is GET /v1/cards/{id}/activity: the card's activity, newest first, paged the same
// way, and filtered to one kind when `kind` is given. A kind that is not one of the fixed list is
// an invalid_argument error, so a client that misspells one is told instead of shown nothing.
func (s *Server) listActivity(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	kind, err := activityKindFilter(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	limit, position, err := readPage(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	page, err := s.history.Activity(r.Context(), id, kind, position, limit)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	answer, err := historyPage(page.Items, page.Cursor, page.More, s.now())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}

// getMessage is GET /v1/cards/{id}/messages/{messageId}: one message in full, for the tool detail
// a chat block opens on demand (N13). A message that is not on this card, or an id that cannot
// exist, is not found.
func (s *Server) getMessage(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	messageID := r.PathValue("messageId")
	if len(messageID) > maxEchoedIDBytes {
		// No stored id is this long, and a very long address must not be repeated in the answer.
		s.writeError(w, notFoundID("message", messageID))
		return
	}
	detail, err := s.history.MessageDetail(r.Context(), id, messageID)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, detail)
}

// readPage reads the paging parameters of a history route: how many items the page holds, and the
// sequence the page before it ended at (zero for the newest page). The rules of both are the ones
// every list route follows (architecture.md 11.5).
func readPage(r *http.Request) (limit int, position int64, err error) {
	limit, raw, err := ParsePage(r)
	if err != nil {
		return 0, 0, err
	}
	cursor, _, err := DecodeCursor[historyCursor](raw)
	if err != nil {
		return 0, 0, err
	}
	return limit, cursor.Seq, nil
}

// historyPage builds the answer of a paged history route. The next cursor is encoded only when
// another page follows, so a client stops at the end instead of asking once more for nothing.
func historyPage[T any](items []T, cursor int64, more bool, now time.Time) (protocol.Page[T], error) {
	next := ""
	if more {
		encoded, err := EncodeCursor(historyCursor{Seq: cursor})
		if err != nil {
			return protocol.Page[T]{}, err
		}
		next = encoded
	}
	return protocol.NewPage(items, next, now), nil
}

// activityKindFilter reads the optional `kind` parameter of the activity route. Empty means every
// kind.
func activityKindFilter(r *http.Request) (protocol.ActivityKind, error) {
	text := r.URL.Query().Get("kind")
	if text == "" {
		return "", nil
	}
	kind := protocol.ActivityKind(text)
	if !kind.Valid() {
		return "", protocol.InvalidArgument("That is not a kind of activity. Choose one of: " +
			strings.Join(activityKindNames(), ", ") + ".")
	}
	return kind, nil
}

// activityKindNames lists the kinds an activity filter takes, in the order of the fixed list, for
// the sentence a refused filter gets.
func activityKindNames() []string {
	kinds := protocol.ActivityKindValues()
	names := make([]string, len(kinds))
	for i, kind := range kinds {
		names[i] = string(kind)
	}
	return names
}
