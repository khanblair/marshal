package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The wire shape of a local CI run (docs/architecture.md sections 9 and 11.1,
// docs/backend-checklist.md B6.5, build-plan 6.5, docs/marshal-product-scope.md 15.3): the project's
// own workflow files, read one step at a time, with what Marshal ran in the card's worktree and what
// it refused to run. A step Marshal will not run is reported with a sentence, never passed over.

// sampleLocalCI is a run with all four step shapes in use: a test step that ran and failed, a lint
// step that was not reached because the test before it failed, a build step that passed, and a
// deploy step Marshal refused to run.
func sampleLocalCI() protocol.LocalCIResult {
	return protocol.NewLocalCIResult(
		sampleCardID,
		"marshal/card-12-fix-the-report",
		[]protocol.LocalCIWorkflow{
			{
				File:   ".github/workflows/ci.yml",
				Name:   "CI",
				Status: protocol.LocalCIStatusFailed,
				Steps: []protocol.LocalCIStep{
					{
						Job: "test", Name: "Run the tests", Kind: protocol.LocalCIKindTest,
						Status:  protocol.LocalCIStatusFailed,
						Command: "pnpm install && pnpm test",
						Output:  "FAIL src/util.test.ts\n  add() adds two numbers\n  1 test failed",
						TookMs:  4210,
					},
					{
						Job: "test", Name: "Typecheck", Kind: protocol.LocalCIKindLint,
						Status:  protocol.LocalCIStatusSkipped,
						Reason:  "An earlier step in this job failed, so this step did not run.",
						Command: "pnpm exec tsc --noEmit",
					},
					{
						Job: "build", Name: "Build the packages", Kind: protocol.LocalCIKindBuild,
						Status: protocol.LocalCIStatusPassed, Command: "pnpm build",
						Output: "built in 3.1s", TookMs: 3120,
					},
					{
						Job: "release", Name: "Publish to npm", Kind: protocol.LocalCIKindOther,
						Status:  protocol.LocalCIStatusUnsupported,
						Reason:  "This step deploys or publishes something, so Marshal did not run it here.",
						Command: "pnpm publish --access public",
					},
				},
			},
		},
		providersNow,
	)
}

func TestLocalCIGolden(t *testing.T) {
	testutil.Golden(t, "local-ci-result", sampleLocalCI())
}

// The four kinds and the four statuses are the words the wire carries, and the app's checks list is
// drawn from them. A fifth value, or a renamed one, would cross the wire without anything noticing.
func TestLocalCICarriesTheFourKindsAndFourStatuses(t *testing.T) {
	kinds := protocol.LocalCIKindValues()
	wantKinds := []protocol.LocalCIKind{
		protocol.LocalCIKindTest, protocol.LocalCIKindLint, protocol.LocalCIKindBuild, protocol.LocalCIKindOther,
	}
	if len(kinds) != len(wantKinds) {
		t.Fatalf("LocalCIKindValues = %v, want %v", kinds, wantKinds)
	}
	for i, want := range wantKinds {
		if kinds[i] != want {
			t.Fatalf("LocalCIKindValues = %v, want %v", kinds, wantKinds)
		}
	}
	if !protocol.LocalCIKindTest.Valid() || protocol.LocalCIKind("compile").Valid() {
		t.Error("LocalCIKind.Valid accepts the wrong words")
	}

	statuses := protocol.LocalCIStatusValues()
	wantStatuses := []protocol.LocalCIStatus{
		protocol.LocalCIStatusPassed, protocol.LocalCIStatusFailed,
		protocol.LocalCIStatusUnsupported, protocol.LocalCIStatusSkipped,
	}
	if len(statuses) != len(wantStatuses) {
		t.Fatalf("LocalCIStatusValues = %v, want %v", statuses, wantStatuses)
	}
	for i, want := range wantStatuses {
		if statuses[i] != want {
			t.Fatalf("LocalCIStatusValues = %v, want %v", statuses, wantStatuses)
		}
	}
	if !protocol.LocalCIStatusUnsupported.Valid() || protocol.LocalCIStatus("errored").Valid() {
		t.Error("LocalCIStatus.Valid accepts the wrong words")
	}
}

// A run with a worktree that has no workflow files answers an empty list, not a null: a screen draws
// "no checks here" without a special case for a missing list.
func TestAnEmptyLocalCIRunCarriesEmptyLists(t *testing.T) {
	result := protocol.NewLocalCIResult(sampleCardID, "main", nil, providersNow)
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"workflows":[]`) {
		t.Errorf("a run with no workflows encoded as %s, want []", body)
	}
	if !strings.Contains(string(body), `"serverTime":"2026-09-27T09:30:00.000Z"`) {
		t.Errorf("an answer carries the daemon's time: %s", body)
	}
	// A workflow's steps are a list too, so a workflow with no steps never encodes a null either.
	withEmpty := protocol.NewLocalCIResult(sampleCardID, "main",
		[]protocol.LocalCIWorkflow{{File: ".github/workflows/empty.yml", Name: "Empty"}}, providersNow)
	body, err = json.Marshal(withEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"steps":[]`) {
		t.Errorf("a workflow with no steps encoded as %s, want []", body)
	}
}
