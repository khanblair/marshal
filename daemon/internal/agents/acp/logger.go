package acp

import (
	"context"
	"log/slog"
)

// rawKey is the attribute under which the SDK logs a whole line of what the agent sent. That line
// can hold a prompt or file contents, and logs must not.
const rawKey = "raw"

// quietHandler is the handler for the protocol library's own diagnostics. It passes on warnings
// and errors only, and removes the raw message text from them.
type quietHandler struct {
	next slog.Handler
}

// connectionLogger wraps a logger for the protocol connection.
func connectionLogger(l *slog.Logger) *slog.Logger {
	return slog.New(quietHandler{next: l.Handler()})
}

func (h quietHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn && h.next.Enabled(ctx, level)
}

func (h quietHandler) Handle(ctx context.Context, r slog.Record) error {
	clean := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key != rawKey {
			clean.AddAttrs(a)
		}
		return true
	})
	return h.next.Handle(ctx, clean)
}

func (h quietHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return quietHandler{next: h.next.WithAttrs(attrs)}
}

func (h quietHandler) WithGroup(name string) slog.Handler {
	return quietHandler{next: h.next.WithGroup(name)}
}
