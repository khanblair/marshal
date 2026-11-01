package cardhistory

import (
	"context"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// run is a stretch of consecutive agent events, or of consecutive thought events, that is read as
// one message. The agent streams its words in many small pieces and each one is stored as an event
// of its own, but a reader wants the reply, not the pieces.
//
// The history is walked newest first, so the pieces arrive last piece first. parts holds them in
// that order and message is the oldest piece read so far, which gives the reply its id, its
// sequence, and its time.
type run struct {
	kind    history.Kind
	message protocol.ChatMessage
	parts   []string
}

// mergeable says whether events of a kind are pieces of one message when they follow each other.
func mergeable(kind history.Kind) bool {
	return kind == history.KindAgent || kind == history.KindThought
}

func newRun(ev history.Event, msg protocol.ChatMessage) *run {
	return &run{kind: ev.Kind, message: msg, parts: []string{msg.Text}}
}

// join adds an older piece to the run.
func (r *run) join(msg protocol.ChatMessage) {
	r.message = msg
	r.parts = append(r.parts, msg.Text)
}

// whole is the reply: the pieces in the order they were sent, under the oldest piece's identity.
func (r *run) whole() protocol.ChatMessage {
	var b strings.Builder
	for i := len(r.parts) - 1; i >= 0; i-- {
		b.WriteString(r.parts[i])
	}
	whole := r.message
	whole.Text = b.String()
	return whole
}

// fillMessages is fill for a card's or a chat's messages: it reads the same pages, newest first,
// but a run of agent pieces is one message, so a page of 50 holds 50 messages and not one reply cut
// to 50 pieces. A run is never split between pages: the walk goes on reading until the run ends,
// and the next page starts below its oldest piece.
func fillMessages(
	ctx context.Context, src pager, cursor int64, limit int,
	convert func(history.Event) (protocol.ChatMessage, bool, error),
) (Page[protocol.ChatMessage], error) {
	w := &messageWalk{page: Page[protocol.ChatMessage]{Items: []protocol.ChatMessage{}, Cursor: cursor}, limit: limit}
	scan := cursor
	budget := max(limit*scanBudgetFactor, minScanBudget)
	for {
		chunk, err := src(ctx, scan, min(max(limit-len(w.page.Items), minChunk), maxStoreChunk))
		if err != nil {
			return Page[protocol.ChatMessage]{}, err
		}
		budget -= len(chunk.Events)
		full, err := w.read(chunk, convert)
		if err != nil {
			return Page[protocol.ChatMessage]{}, err
		}
		if full {
			return w.page, nil
		}
		if !chunk.More {
			w.flush()
			if len(w.page.Items) == 0 {
				w.page.Cursor = cursor
			}
			return w.page, nil
		}
		if budget <= 0 && w.open == nil {
			w.page.Cursor, w.page.More = chunk.Cursor, true
			return w.page, nil
		}
		scan = chunk.Cursor
	}
}

// messageWalk is the state of one fillMessages call: the page so far, and the run of agent pieces
// that has not ended yet.
type messageWalk struct {
	page  Page[protocol.ChatMessage]
	limit int
	open  *run
}

// flush closes the open run into the page as one message.
func (w *messageWalk) flush() {
	if w.open == nil {
		return
	}
	w.page.Items = append(w.page.Items, w.open.whole())
	w.page.Cursor = w.open.message.Seq
	w.open = nil
}

// read adds one chunk's events, newest first. It answers true when the page is full.
func (w *messageWalk) read(
	chunk history.Page, convert func(history.Event) (protocol.ChatMessage, bool, error),
) (bool, error) {
	for i, ev := range chunk.Events {
		if w.open != nil && ev.Kind != w.open.kind {
			w.flush()
			if len(w.page.Items) == w.limit {
				// The event that ended the run is not read: the next page starts with it.
				w.page.More = true
				return true, nil
			}
		}
		msg, keep, err := convert(ev)
		if err != nil {
			return false, err
		}
		if !keep {
			continue
		}
		if w.open != nil {
			w.open.join(msg)
			continue
		}
		if mergeable(ev.Kind) {
			w.open = newRun(ev, msg)
			continue
		}
		w.page.Items = append(w.page.Items, msg)
		w.page.Cursor = ev.Seq
		if len(w.page.Items) == w.limit {
			w.page.More = i < len(chunk.Events)-1 || chunk.More
			return true, nil
		}
	}
	return false, nil
}
