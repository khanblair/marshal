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
	page := Page[protocol.ChatMessage]{Items: []protocol.ChatMessage{}, Cursor: cursor}
	scan := cursor
	budget := max(limit*scanBudgetFactor, minScanBudget)
	var open *run
	flush := func() {
		if open == nil {
			return
		}
		page.Items = append(page.Items, open.whole())
		page.Cursor = open.message.Seq
		open = nil
	}
	for {
		chunk, err := src(ctx, scan, min(max(limit-len(page.Items), minChunk), maxStoreChunk))
		if err != nil {
			return Page[protocol.ChatMessage]{}, err
		}
		budget -= len(chunk.Events)
		for i, ev := range chunk.Events {
			if open != nil && ev.Kind != open.kind {
				flush()
				if len(page.Items) == limit {
					// The event that ended the run is not read: the next page starts with it.
					page.More = true
					return page, nil
				}
			}
			msg, keep, err := convert(ev)
			if err != nil {
				return Page[protocol.ChatMessage]{}, err
			}
			if !keep {
				continue
			}
			if open != nil {
				open.join(msg)
				continue
			}
			if mergeable(ev.Kind) {
				open = newRun(ev, msg)
				continue
			}
			page.Items = append(page.Items, msg)
			page.Cursor = ev.Seq
			if len(page.Items) == limit {
				page.More = i < len(chunk.Events)-1 || chunk.More
				return page, nil
			}
		}
		if !chunk.More {
			flush()
			if len(page.Items) == 0 {
				page.Cursor = cursor
			}
			return page, nil
		}
		if budget <= 0 && open == nil {
			page.Cursor, page.More = chunk.Cursor, true
			return page, nil
		}
		scan = chunk.Cursor
	}
}
