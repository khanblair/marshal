package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Session search is the second half of what docs/architecture.md section 10 asks for: the daemon
// keeps one line for each moment of a session, and a search reads those lines through the full-text
// index migration 0021 builds. It lives beside the note search because section 3 gives both to the
// memory module; these tests are about what the search answers, not about how the index is kept
// (that is internal/store's TestTheSessionIndexFollowsTheEvent).
//
// Everything real is real here: the events are rows in a temporary database, written the way the
// session manager writes them. Only the cards are the fixture's fake, as everywhere in this file.

// addEvent stores one event of a card's session: the session row first, since an event hangs from
// it, then the event itself, numbered through the same query the session manager uses. at is when
// the event happened and is what orders two equally good matches.
func (f *fixture) addEvent(t *testing.T, id, card, summary string, at time.Time) {
	t.Helper()
	ctx := context.Background()
	err := f.store.Write(ctx, func(q *db.Queries) error {
		sessionID := "session-" + card
		if _, err := q.GetSessionByCard(ctx, card); err != nil {
			if !store.IsNotFound(err) {
				return err
			}
			if err := q.CreateCardSession(ctx, db.CreateCardSessionParams{
				ID: sessionID, CardID: card, AgentKind: "claude", State: "idle",
				LastActiveAt: at.UnixMilli(), CreatedAt: at.UnixMilli(), UpdatedAt: at.UnixMilli(),
			}); err != nil {
				return err
			}
		}
		seq, err := q.NextSessionEventSeq(ctx, card)
		if err != nil {
			return err
		}
		return q.InsertSessionEvent(ctx, db.InsertSessionEventParams{
			ID: id, CardID: card, SessionID: sessionID, Seq: seq,
			Kind: "message", Summary: summary, CreatedAt: at.UnixMilli(),
		})
	})
	if err != nil {
		t.Fatalf("store the session event %s: %v", id, err)
	}
}

func TestSearchSessionsFindsAStoredEventByAPrefixOfAWord(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.addEvent(t, "event-1", cardID, "Added a health check endpoint", testNow)
	// An event of another project with the same words, which this project's search must not answer.
	f.addEvent(t, "event-2", awayCard, "a health check elsewhere", testNow)

	found, err := f.svc.SearchSessions(ctx, projectID, "heal", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("a prefix search found %d events, want this project's one", len(found))
	}
	hit := found[0]
	if hit.CardID != cardID || hit.Key != "small-repo#7" || hit.Title != "Add a health check" {
		t.Errorf("the hit names card %q key %q title %q, want this project's #7", hit.CardID, hit.Key, hit.Title)
	}
	// The excerpt is the stored summary as it was written: a summary is already one line, so there
	// is nothing to cut.
	if hit.Excerpt != "Added a health check endpoint" {
		t.Errorf("excerpt = %q, want the stored summary", hit.Excerpt)
	}
	if hit.ProjectID != projectID {
		t.Errorf("project = %q, want %q", hit.ProjectID, projectID)
	}
	// The project's name is the caller's to fill: the search already knows which project it is
	// walking, so a second read of it here would be for nothing.
	if hit.ProjectName != "" {
		t.Errorf("project name = %q, want it left to the caller", hit.ProjectName)
	}
	if !hit.At.Time().Equal(testNow) {
		t.Errorf("the hit is stamped %v, want the event's own %v", hit.At, testNow)
	}

	for _, tc := range []struct {
		name  string
		query string
	}{
		{"a word nobody wrote", "pineapple"},
		{"no words at all", "   ---  "},
		{"nothing at all", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.svc.SearchSessions(ctx, projectID, tc.query, 10)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("search(%q) answered %d events, want none", tc.query, len(got))
			}
		})
	}
}

// Equally good matches come back newest first, so the moment a person is looking for is the one
// they most recently lived. The index orders by its own relevance and the event's time breaks a tie.
func TestSearchSessionsAnswersTheNewestOfTwoEquals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.addEvent(t, "older", cardID, "the health check ran", testNow)
	f.addEvent(t, "newer", sibling, "the health check ran", testNow.Add(time.Hour))

	found, err := f.svc.SearchSessions(ctx, projectID, "health", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("found %d events, want both", len(found))
	}
	if found[0].CardID != sibling || found[1].CardID != cardID {
		t.Errorf("the events came back as %q then %q, want the newer card first",
			found[0].CardID, found[1].CardID)
	}
}

func TestSearchSessionsAnswersAtMostTheLimitItIsGiven(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for i, id := range []string{cardID, sibling, third} {
		f.addEvent(t, "event-"+id, id, "the widget report", testNow.Add(time.Duration(i)*time.Minute))
	}
	found, err := f.svc.SearchSessions(ctx, projectID, "widget", 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("search with a limit of 2 answered %d events", len(found))
	}
	// A limit of zero means "the caller did not say", which is the default and finds all three.
	all, err := f.svc.SearchSessions(ctx, projectID, "widget", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("search with no limit answered %d events, want 3", len(all))
	}
}

// An event whose card the projects service does not know about is skipped rather than failing the
// search: the row's own key should not have allowed it, and one unnameable event is not worth
// refusing an answer over. The same rule a note search follows.
func TestSearchSessionsSkipsAnEventWhoseCardIsGone(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.addEvent(t, "vanished", cardID, "the health check ran", testNow)
	f.addEvent(t, "kept", sibling, "the health check ran too", testNow)
	delete(f.cards.cards, cardID)

	found, err := f.svc.SearchSessions(ctx, projectID, "health", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 1 || found[0].CardID != sibling {
		t.Fatalf("found %+v, want only the card that is still there", found)
	}
}

// The answer is named by the card's own fields, which are read once per hit from the projects
// service: what a palette opens on a hit is the card, and the event stores only the card's id.
func TestSearchSessionsNamesEachHitByItsCard(t *testing.T) {
	f := newFixture(t)
	f.addEvent(t, "event-1", sibling, "trimmed the bundle by a third", testNow)
	hit := f.searchOne(t, "bundle")
	if hit.CardID != sibling || hit.Key != "small-repo#8" || hit.Title != "Fix the retry loop" {
		t.Errorf("hit = %+v, want card #8 keyed small-repo#8", hit)
	}
}

// searchOne runs a session search that must find exactly one event, and answers it.
func (f *fixture) searchOne(t *testing.T, query string) protocol.SessionHit {
	t.Helper()
	found, err := f.svc.SearchSessions(context.Background(), projectID, query, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("search(%q) found %d events, want 1", query, len(found))
	}
	return found[0]
}
