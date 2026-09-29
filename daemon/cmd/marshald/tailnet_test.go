package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// openTestStore opens a store of its own in a temporary folder, closed when the test ends.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// A daemon that was never asked to join a tailnet has no node at all - not a node that failed, and
// not a node that is switched off. Closing nothing is not an error, which is what lets serve defer
// the close unconditionally.
func TestThereIsNoTailnetNodeUnlessItIsAskedFor(t *testing.T) {
	st := openTestStore(t)
	settings := config.Settings{Mode: platform.ModeNormal, DataDir: t.TempDir()}
	if node := buildTailnetNode(t.Context(), settings, st, discardLog()); node != nil {
		t.Fatalf("node = %v, want nil for a daemon that did not ask for a tailnet", node)
	}
	var err error
	closeTailnet(nil, &err)
	if err != nil {
		t.Errorf("closing no node = %v, want nil", err)
	}
}

// A daemon that did ask for one gets a node that says so, before it has joined anything: it is
// enabled, it is signing in, and it carries the name it was given. Building one touches no
// network and writes no file - nothing is joined until the server asks it to be.
func TestAAskedForTailnetNodeIsBuiltAndSaysSo(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()
	settings := config.Settings{
		Mode: platform.ModeNormal, DataDir: dir, Tailnet: true, TailnetHostname: "marshal",
	}
	ctx, cancel := context.WithCancel(t.Context())
	node := buildTailnetNode(ctx, settings, st, discardLog())
	t.Cleanup(cancel)
	if node == nil {
		t.Fatal("node = nil, want one: the daemon was asked to join a tailnet")
	}
	status := node.Status()
	if !status.Enabled || status.State != "signing-in" || status.Hostname != "marshal" {
		t.Errorf("status = %+v, want an enabled node signing in as marshal", status)
	}
	if status.DNSName != "" || status.Identity != "" || status.Error != "" {
		t.Errorf("status = %+v, want nothing claimed before anything has happened", status)
	}
	var err error
	closeTailnet(node, &err)
	if err != nil {
		t.Errorf("close the node = %v, want nil", err)
	}
}

// fakeTailnetStatus answers with whatever a test says the node is.
type fakeTailnetStatus struct{ status protocol.TailnetStatus }

func (f fakeTailnetStatus) Status() protocol.TailnetStatus { return f.status }

// The account the node joined as lands on the owner's row, so the profile's Tailscale identity is
// read from the node rather than typed in. A node with no identity yet writes nothing, and the
// watcher stops when the daemon does.
func TestTheTailnetIdentityIsWrittenWhenTheNodeHasOne(t *testing.T) {
	st := openTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node := fakeTailnetStatus{status: protocol.TailnetStatus{State: "online", Identity: "blair@example.com"}}
	go watchTailnetIdentity(ctx, node, st, discardLog())

	ownerID := waitForOwnerID(t, st, "blair@example.com")
	owner, err := st.Queries().GetOwner(context.Background())
	if err != nil {
		t.Fatalf("read the owner: %v", err)
	}
	if owner.ID != ownerID {
		t.Errorf("owner id = %q, want the one that was written to", owner.ID)
	}
	if owner.TailnetIdentity != "blair@example.com" {
		t.Errorf("tailnet identity = %q, want the account the node joined as", owner.TailnetIdentity)
	}
}

// A node that has not joined has no account, and a row that has none stays none: nothing is
// guessed and no empty string is written over the value a person already has.
func TestATailnetThatHasNotJoinedWritesNoIdentity(t *testing.T) {
	st := openTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watchTailnetIdentity(ctx, fakeTailnetStatus{}, st, discardLog())

	time.Sleep(50 * time.Millisecond)
	owner, err := st.Queries().GetOwner(context.Background())
	if err != nil {
		t.Fatalf("read the owner: %v", err)
	}
	if owner.TailnetIdentity != "" {
		t.Errorf("tailnet identity = %q, want none written by a node that has not joined", owner.TailnetIdentity)
	}
}

// waitForOwnerID polls until the owner's row carries the identity, and fails the test if it never
// does. The watcher writes on its first pass, so this is a matter of milliseconds and not of the
// interval it polls on afterwards.
func waitForOwnerID(t *testing.T, st *store.Store, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		owner, err := st.Queries().GetOwner(context.Background())
		if err == nil && owner.TailnetIdentity == want {
			return owner.ID
		}
		time.Sleep(5 * time.Millisecond)
	}
	owner, err := st.Queries().GetOwner(context.Background())
	if err != nil {
		t.Fatalf("read the owner: %v", err)
	}
	t.Fatalf("tailnet identity = %q, want %q to have been written", owner.TailnetIdentity, want)
	return ""
}
