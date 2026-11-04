package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/builtin"
	"github.com/khanblair/marshal/daemon/internal/providers"
)

type historyResolver struct{}

func (historyResolver) Resolve(string) (builtin.Resolved, error) {
	return builtin.Resolved{ProviderID: "fixture", Model: "fixture"}, nil
}

func TestBuiltinFactoryUsesTheProductionHistoryReader(t *testing.T) {
	read := false
	factory := builtinAgentFactory(slog.New(slog.NewTextHandler(io.Discard, nil)), historyResolver{}, func(ctx context.Context, id string) ([]providers.Message, error) {
		read = true
		if id != "saved-native-id" || ctx != t.Context() {
			t.Fatalf("wrong resume identity or context: %s", id)
		}
		return []providers.Message{{Role: providers.RoleUser, Parts: []providers.Part{{Kind: providers.PartText, Text: "earlier context"}}}}, nil
	})
	a, err := factory()
	if err != nil {
		t.Fatal(err)
	}
	h, err := a.Resume(t.Context(), "saved-native-id", agents.StartSpec{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Stop(context.Background(), h) }()
	if !read || h.ID != "saved-native-id" {
		t.Fatal("production factory lost the reader or changed the conversation ID")
	}
}
