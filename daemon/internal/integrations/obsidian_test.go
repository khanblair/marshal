package integrations_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The Obsidian vault connection (docs/architecture.md section 18, build-plan task 7.7): its test
// and its row's three states. Unlike GitHub, nobody sets this connection up, so these tests drive it
// entirely through the vault folder on disk - present and writable, missing, or present and
// read-only - and through what fixture.saveTest below writes straight into the row, the same two
// columns internal/connectiontest.Runner writes after a real test (docs/store/queries/
// integrations.sql's SetIntegrationTest), so a test here is reading what the screen would read.

// saveTest writes result as the connection's own last test, the way the real Runner does after
// running one, so List reads back a test this file ran without going through the route layer or a
// second daemon-wide Runner. It is a method on fixture rather than a free function so it reads like
// fixture.connected does above: fixture's own way of putting the store in a state a test starts from.
func (f *fixture) saveTest(id, kind string, result protocol.TestResult) {
	f.t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		f.t.Fatalf("encode the test result: %v", err)
	}
	err = f.store.Write(context.Background(), func(q *db.Queries) error {
		return q.SetIntegrationTest(context.Background(), db.SetIntegrationTestParams{
			ID: id, Kind: kind, LastTestAt: result.RanAt.Time().UnixMilli(), LastTestResultJSON: string(encoded),
		})
	})
	if err != nil {
		f.t.Fatalf("save the test result: %v", err)
	}
}

// obsidianRow finds the Obsidian connection in a List answer, so a test can read its status without
// depending on which position "obsidian" happens to hold in the screen's own order.
func obsidianRow(t *testing.T, list []protocol.Integration) protocol.Integration {
	t.Helper()
	for _, row := range list {
		if row.ID == integrations.ObsidianID {
			return row
		}
	}
	t.Fatalf("List did not answer an obsidian row at all")
	return protocol.Integration{}
}

// A vault folder that is there and writable is a passed test, and the row reads connected even
// before any test is run - the folder alone is what vaultStatus asks about.
func TestObsidianConnectedWhenTheVaultExistsAndIsWritable(t *testing.T) {
	vault := t.TempDir()
	f := newFixture(t, func(o *integrations.Options) { o.VaultRoot = vault })

	result, err := f.svc.Test(context.Background(), integrations.ObsidianID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.OK {
		t.Errorf("a writable vault answered OK=false, checks: %+v", result.Checks)
	}
	for _, check := range result.Checks {
		if check.State == protocol.CheckStateFailed {
			t.Errorf("check %q failed on a writable vault: %s", check.Name, check.Message)
		}
	}

	list, err := f.svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	row := obsidianRow(t, list)
	if row.Status != protocol.IntegrationStatusConnected {
		t.Errorf("the row reads %q with a writable vault and no test run, want connected", row.Status)
	}
}

// A vault folder that does not exist yet is not a failure: Marshal makes it when the first note is
// saved, so the test warns rather than fails, and a warning never turns the row's own "connected"
// (the folder will be there, the sentence says so) into "needs attention".
func TestObsidianWarnsWithoutFailingWhenTheVaultIsMissing(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "vault")
	f := newFixture(t, func(o *integrations.Options) { o.VaultRoot = vault })

	result, err := f.svc.Test(context.Background(), integrations.ObsidianID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.OK {
		t.Errorf("a missing vault answered OK=false, want a warning that still counts as OK: %+v", result.Checks)
	}
	sawWarning := false
	for _, check := range result.Checks {
		if check.Name == integrations.CheckVaultFolder {
			if check.State != protocol.CheckStateWarning {
				t.Errorf("the folder check on a missing vault is %q, want warning", check.State)
			}
			sawWarning = true
		}
		if check.State == protocol.CheckStateFailed {
			t.Errorf("check %q failed on a missing vault, want no failures", check.Name)
		}
	}
	if !sawWarning {
		t.Fatalf("no %q check in the answer: %+v", integrations.CheckVaultFolder, result.Checks)
	}

	f.saveTest(integrations.ObsidianID, integrations.KindObsidian, result)
	list, err := f.svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	row := obsidianRow(t, list)
	if row.Status != protocol.IntegrationStatusConnected {
		t.Errorf("the row reads %q after a warning-only test, want connected", row.Status)
	}
	if row.LastTest == nil {
		t.Error("the row has no last test after one was saved")
	}
}

// A vault folder that is there but cannot be written to is a real failure, and a saved test that
// found one turns the row to "needs attention" (error): rowToWire's self-owned branch trusts the
// last test's own OK over what the folder's mere existence would otherwise say.
func TestObsidianNeedsAttentionWhenTheLastTestFailed(t *testing.T) {
	vault := t.TempDir()
	if err := os.Chmod(vault, 0o500); err != nil {
		t.Fatalf("make the vault read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(vault, 0o700) })
	f := newFixture(t, func(o *integrations.Options) { o.VaultRoot = vault })

	result, err := f.svc.Test(context.Background(), integrations.ObsidianID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.OK {
		t.Fatalf("a read-only vault answered OK=true, checks: %+v", result.Checks)
	}
	failed := false
	for _, check := range result.Checks {
		if check.Name == integrations.CheckVaultWritable && check.State == protocol.CheckStateFailed {
			failed = true
		}
	}
	if !failed {
		t.Fatalf("no failed %q check in the answer: %+v", integrations.CheckVaultWritable, result.Checks)
	}

	f.saveTest(integrations.ObsidianID, integrations.KindObsidian, result)
	list, err := f.svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	row := obsidianRow(t, list)
	if row.Status != protocol.IntegrationStatusError {
		t.Errorf("the row reads %q after a failed test, want error (needs attention)", row.Status)
	}
	if row.Detail == "" {
		t.Error("a needs-attention row has no detail saying what to fix")
	}
}
