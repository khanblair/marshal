package agentprefs

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

func serviceFor(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "marshal.db"), store.WithLogger(nil))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc, err := New(st)
	if err != nil {
		t.Fatalf("make the agent settings: %v", err)
	}
	return svc
}

// An agent nobody set answers the default, and a saved choice is read back as it was saved.
func TestAChoiceIsReadBackAndTheDefaultIsWhatIsUnset(t *testing.T) {
	svc := serviceFor(t)
	ctx := context.Background()

	unset, err := svc.Preference(ctx, protocol.AgentKindClaude)
	if err != nil || unset != (catalog.Preference{}) {
		t.Fatalf("unset claude = %+v, %v; want the default", unset, err)
	}
	want := catalog.Preference{Path: "/Users/pat/bin/claude", Off: true}
	if err := svc.SetPreference(ctx, protocol.AgentKindClaude, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := svc.Preference(ctx, protocol.AgentKindClaude)
	if err != nil || got != want {
		t.Errorf("claude = %+v, %v; want %+v", got, err, want)
	}
	other, _ := svc.Preference(ctx, protocol.AgentKindGemini)
	if other != (catalog.Preference{}) {
		t.Errorf("gemini = %+v, want the default: a choice belongs to its agent only", other)
	}
}
