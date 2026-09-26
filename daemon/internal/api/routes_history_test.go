package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The card chat and activity routes (docs/backend-checklist.md B2.6 and B2.8). They read the same
// stored history the session manager writes, so a page is a real page of a real card.

// addHistory stores records in a card's history. A card that no session has run for has no session
// row, and every stored event points at one, so the row is made first.
func addHistory(t *testing.T, st *stack, cardID string, records ...history.Record) {
	t.Helper()
	ctx := context.Background()
	session, err := st.store.Queries().GetSessionByCard(ctx, cardID)
	if store.IsNotFound(err) {
		now := time.Now().UTC()
		session.ID = "session-" + cardID
		err = st.store.Write(ctx, func(q *db.Queries) error {
			return q.CreateCardSession(ctx, db.CreateCardSessionParams{
				ID: session.ID, CardID: cardID, AgentKind: "claude", State: "awake",
				LastActiveAt: now.UnixMilli(), CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(),
			})
		})
	}
	if err != nil {
		t.Fatalf("make the session row of card %s: %v", cardID, err)
	}
	if err := st.hist.Append(ctx, cardID, session.ID, records); err != nil {
		t.Fatalf("store the history of card %s: %v", cardID, err)
	}
}

func TestCardMessagesThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Upgrade the library")
	addHistory(t, st, card.ID,
		history.Record{Kind: history.KindUser, Summary: "Upgrade the upstream library."},
		history.Record{Kind: history.KindToolCall, Summary: "Edited internal/upstream/conn.go", State: history.StateOK,
			Detail: `{"id":"call_1","title":"Edited internal/upstream/conn.go","toolKind":"edit",` +
				`"path":"internal/upstream/conn.go","content":"Replaced Dial.","diffs":[{"path":"internal/upstream/conn.go","oldText":"a","newText":"b"}]}`},
		history.Record{Kind: history.KindAgent, Summary: "I read the callers."},
	)
	path := "/v1/cards/" + card.ID + "/messages"

	got := st.do(http.MethodGet, path, nil).want(t, http.StatusOK)
	sameShape(t, "chat-messages", got.Body)
	page := decode[protocol.Page[protocol.ChatMessage]](t, got)
	if len(page.Items) != 3 {
		t.Fatalf("the page has %d messages, want 3: %+v", len(page.Items), page.Items)
	}
	if page.Items[0].Kind != protocol.ChatMessageKindAgent || page.Items[0].Text != "I read the callers." {
		t.Errorf("the newest message = %+v, want the agent's answer", page.Items[0])
	}
	if page.Items[1].Kind != protocol.ChatMessageKindTool || page.Items[1].Tool == nil {
		t.Fatalf("the second message = %+v, want the tool call", page.Items[1])
	}
	if page.Items[1].Tool.Title != "Edited internal/upstream/conn.go" || !page.Items[1].Tool.HasDetail {
		t.Errorf("the tool call = %+v, want its line and its detail", *page.Items[1].Tool)
	}
	if page.Items[1].Diff != nil {
		t.Error("a tool message carries a diff summary, which the detail route owns")
	}
	if page.Items[2].Kind != protocol.ChatMessageKindUser || page.Items[2].Text != "Upgrade the upstream library." {
		t.Errorf("the oldest message = %+v", page.Items[2])
	}
	if page.NextCursor != "" {
		t.Errorf("nextCursor = %q, want empty at the end", page.NextCursor)
	}
	if page.ServerTime.Time().IsZero() {
		t.Error("the page has no serverTime")
	}
}

func TestCardMessagesPageByCursor(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Upgrade the library")
	addHistory(t, st, card.ID,
		history.Record{Kind: history.KindUser, Summary: "one"},
		history.Record{Kind: history.KindAgent, Summary: "two"},
		history.Record{Kind: history.KindAgent, Summary: "three"},
	)
	path := "/v1/cards/" + card.ID + "/messages"

	first := decode[protocol.Page[protocol.ChatMessage]](t, st.do(http.MethodGet, path+"?limit=2", nil).want(t, http.StatusOK))
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("the first page = %+v, want two messages and a cursor", first)
	}
	if first.Items[0].Text != "three" || first.Items[1].Text != "two" {
		t.Errorf("the first page = %+v", first.Items)
	}
	second := decode[protocol.Page[protocol.ChatMessage]](t,
		st.do(http.MethodGet, path+"?limit=2&cursor="+first.NextCursor, nil).want(t, http.StatusOK))
	if len(second.Items) != 1 || second.Items[0].Text != "one" || second.NextCursor != "" {
		t.Fatalf("the second page = %+v, want the last message and no cursor", second)
	}
	third := decode[protocol.Page[protocol.ChatMessage]](t,
		st.do(http.MethodGet, path+"?limit=2&cursor="+first.NextCursor+"&cursor="+first.NextCursor, nil).want(t, http.StatusOK))
	if len(third.Items) != 1 {
		t.Errorf("the same cursor twice = %+v", third.Items)
	}
}

func TestCardMessagesRefuseABadRequest(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Upgrade the library")
	path := "/v1/cards/" + card.ID + "/messages"
	tests := []struct {
		name    string
		query   string
		message string
	}{
		{"a limit that is not a number", "?limit=many", "The page size must be a whole number from 1 to 200. Change the limit and try again."},
		{"a limit of zero", "?limit=0", "The page size must be a whole number from 1 to 200. Change the limit and try again."},
		{"a cursor that cannot be read", "?cursor=!!!", "That page marker is not valid. Go back to the first page and try again."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := st.do(http.MethodGet, path+tc.query, nil).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
			if got.Message != tc.message {
				t.Errorf("message = %q\nwant      %q", got.Message, tc.message)
			}
		})
	}
}

// A card with no history is an empty page, and its list is [] and never null.
func TestCardMessagesOfACardWithNoHistory(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Upgrade the library")

	got := st.do(http.MethodGet, "/v1/cards/"+card.ID+"/messages", nil).want(t, http.StatusOK)
	if !strings.Contains(string(got.Body), `"items":[]`) {
		t.Errorf("the body is %s, want an empty list and not null", got.Body)
	}
	page := decode[protocol.Page[protocol.ChatMessage]](t, got)
	if len(page.Items) != 0 || page.NextCursor != "" {
		t.Errorf("page = %+v, want nothing and no cursor", page)
	}
}

func TestCardMessagesOfAnUnknownCard(t *testing.T) {
	st := newStack(t)
	got := st.do(http.MethodGet, "/v1/cards/01M3C107JB041061050R3GG28A/messages", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	if got.Message != "Marshal cannot find that card. It may have been removed." {
		t.Errorf("message = %q", got.Message)
	}
	if got.Details["id"] != "01M3C107JB041061050R3GG28A" {
		t.Errorf("details = %v, want the id that was asked for", got.Details)
	}
	// An id of the wrong shape can never exist, so it reads the same way.
	st.do(http.MethodGet, "/v1/cards/not-an-id/messages", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

func TestCardActivityThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Upgrade the library")
	addHistory(t, st, card.ID,
		history.Record{Kind: history.KindUser, Summary: "Upgrade the upstream library."},
		history.Record{Kind: history.KindToolCall, Summary: "Edited internal/upstream/conn.go", State: history.StateOK,
			Detail: `{"id":"c1","toolKind":"edit","path":"internal/upstream/conn.go"}`},
		history.Record{Kind: history.KindAgent, Summary: "I read the callers."},
		history.Record{Kind: history.KindToolCall, Summary: "Bash: go test ./...", State: history.StateRunning,
			Detail: `{"id":"c2","toolKind":"execute","command":"go test ./...","content":"231 passed\n"}`},
	)
	path := "/v1/cards/" + card.ID + "/activity"

	got := st.do(http.MethodGet, path, nil).want(t, http.StatusOK)
	sameShape(t, "activity-items", got.Body)
	page := decode[protocol.Page[protocol.ActivityItem]](t, got)
	if len(page.Items) != 2 {
		t.Fatalf("the activity page has %d entries, want 2 (the chat is not activity): %+v", len(page.Items), page.Items)
	}
	if page.Items[0].Kind != protocol.ActivityKindTest || page.Items[0].Result != "231 passed" {
		t.Errorf("the newest entry = %+v, want the test and what it said", page.Items[0])
	}
	if page.Items[1].Kind != protocol.ActivityKindFile || page.Items[1].State != protocol.ActivityStateOK {
		t.Errorf("the older entry = %+v, want the file the tool edited", page.Items[1])
	}

	filtered := decode[protocol.Page[protocol.ActivityItem]](t, st.do(http.MethodGet, path+"?kind=file", nil).want(t, http.StatusOK))
	if len(filtered.Items) != 1 || filtered.Items[0].Kind != protocol.ActivityKindFile {
		t.Errorf("kind=file gave %+v", filtered.Items)
	}
	none := decode[protocol.Page[protocol.ActivityItem]](t, st.do(http.MethodGet, path+"?kind=approval", nil).want(t, http.StatusOK))
	if len(none.Items) != 0 || none.Items == nil {
		t.Errorf("kind=approval gave %+v, want an empty list", none.Items)
	}
}

func TestCardActivityRefusesAnUnknownKind(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Upgrade the library")
	got := st.do(http.MethodGet, "/v1/cards/"+card.ID+"/activity?kind=nonsense", nil).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	want := "That is not a kind of activity. Choose one of: file, command, test, tool, approval."
	if got.Message != want {
		t.Errorf("message = %q\nwant      %q", got.Message, want)
	}
}

func TestCardActivityOfAnUnknownCard(t *testing.T) {
	st := newStack(t)
	st.do(http.MethodGet, "/v1/cards/01M3C107JB041061050R3GG28A/activity", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

// The detail a chat block opens on demand: the page carries the line, and this route carries the
// output and the diffs.
func TestCardMessageDetailThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Upgrade the library")
	addHistory(t, st, card.ID,
		history.Record{Kind: history.KindToolCall, Summary: "Edited internal/upstream/conn.go", State: history.StateOK,
			Detail: `{"id":"call_1","title":"Edited internal/upstream/conn.go","toolKind":"edit",` +
				`"path":"internal/upstream/conn.go","content":"Replaced Dial.","truncated":true,` +
				`"diffs":[{"path":"internal/upstream/conn.go","oldText":"a","newText":"b"}]}`},
	)
	page := decode[protocol.Page[protocol.ChatMessage]](t,
		st.do(http.MethodGet, "/v1/cards/"+card.ID+"/messages", nil).want(t, http.StatusOK))
	messageID := page.Items[0].ID

	got := st.do(http.MethodGet, "/v1/cards/"+card.ID+"/messages/"+messageID, nil).want(t, http.StatusOK)
	sameShape(t, "chat-message-detail", got.Body)
	detail := decode[protocol.ChatMessageDetail](t, got)
	if detail.Tool == nil {
		t.Fatal("the answer carries no tool detail")
	}
	if detail.Tool.Content != "Replaced Dial." || !detail.Tool.Truncated || len(detail.Tool.Diffs) != 1 {
		t.Errorf("tool detail = %+v", *detail.Tool)
	}
	if detail.Message.ID != messageID || detail.Message.Kind != protocol.ChatMessageKindTool {
		t.Errorf("message = %+v", detail.Message)
	}
	if detail.ServerTime.Time().IsZero() {
		t.Error("the answer has no serverTime")
	}
}

func TestCardMessageDetailIsNotFound(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Upgrade the library")
	other := st.addCard(project.ID, "Another card")
	addHistory(t, st, card.ID, history.Record{Kind: history.KindAgent, Summary: "hello"})
	page := decode[protocol.Page[protocol.ChatMessage]](t,
		st.do(http.MethodGet, "/v1/cards/"+card.ID+"/messages", nil).want(t, http.StatusOK))

	for name, path := range map[string]string{
		"an id that is not there":  "/v1/cards/" + card.ID + "/messages/01M3C107JB041061050R3GG28Z",
		"another card's message":   "/v1/cards/" + other.ID + "/messages/" + page.Items[0].ID,
		"a card that is not there": "/v1/cards/01M3C107JB041061050R3GG28A/messages/" + page.Items[0].ID,
	} {
		t.Run(name, func(t *testing.T) {
			st.do(http.MethodGet, path, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
		})
	}
}

// The read path and the write path agree: a message that was really sent to a card's session is on
// the card's history page afterwards.
func TestCardMessagesHoldWhatASessionWrote(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Add a health check")
	base := "/v1/cards/" + card.ID
	stream := st.dial(protocol.CardTopic(card.ID))

	st.do(http.MethodPost, base+"/start", nil).want(t, http.StatusOK)
	stream.until(stateOf(card.ID, protocol.SessionStateAwake))
	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "please add a health check"}).
		want(t, http.StatusNoContent)
	stream.until(stateOf(card.ID, protocol.SessionStateWorking))
	stream.until(stateOf(card.ID, protocol.SessionStateAwake))

	page := decode[protocol.Page[protocol.ChatMessage]](t,
		st.do(http.MethodGet, base+"/messages", nil).want(t, http.StatusOK))
	var texts []string
	for _, message := range page.Items {
		if message.Kind == protocol.ChatMessageKindUser || message.Kind == protocol.ChatMessageKindAgent {
			texts = append(texts, message.Text)
		}
	}
	joined := strings.Join(texts, "")
	if !strings.Contains(joined, "please add a health check") {
		t.Errorf("the history holds %q, want the message that was sent", joined)
	}
	if !strings.Contains(joined, "Turn 1.") {
		t.Errorf("the history holds %q, want the agent's answer", joined)
	}
	st.do(http.MethodPost, base+"/stop", nil).want(t, http.StatusNoContent)
}
