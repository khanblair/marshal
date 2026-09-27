package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The wire shape of a card's live preview (B6.6, N10): where it is, the address it answers on, the
// command that was run, and the before and after screenshots.

// samplePreview is a running preview with both screenshots.
func samplePreview() protocol.Preview {
	running := providersNow.Add(-90 * time.Second)
	taken := providersNow.Add(-30 * time.Second)
	return protocol.Preview{
		CardID:    sampleCardID,
		State:     protocol.PreviewStateRunning,
		URL:       "http://127.0.0.1:5112/reports",
		Port:      5112,
		Command:   "pnpm dev",
		StartedAt: timestampPtr(running),
		Shots: []protocol.PreviewShot{
			{
				Kind:  protocol.PreviewShotKindBefore,
				URL:   "/v1/cards/" + sampleCardID + "/preview/shots/before.png?v=1790488200000",
				Width: 1440, Height: 900,
				TakenAt: protocol.NewTimestamp(taken),
			},
			{
				Kind:  protocol.PreviewShotKindAfter,
				URL:   "/v1/cards/" + sampleCardID + "/preview/shots/after.png?v=1790488260000",
				Width: 1440, Height: 900,
				TakenAt: protocol.NewTimestamp(providersNow),
			},
		},
	}
}

func TestPreviewGolden(t *testing.T) {
	testutil.Golden(t, "preview", samplePreview())
}

func TestPreviewSnapshotGolden(t *testing.T) {
	testutil.Golden(t, "preview-snapshot", protocol.NewPreviewSnapshot(samplePreview(), providersNow))
}

func TestPreviewEventGolden(t *testing.T) {
	testutil.Golden(t, "preview-event", protocol.PreviewEventData{Preview: samplePreview()})
}

// sampleShotResult is what a screenshot that was taken answers: the preview as it now is, with the
// new shot in it, and the sentence saying what was captured.
func sampleShotResult() protocol.PreviewShotResult {
	return protocol.NewPreviewShotResult(samplePreview(), protocol.PreviewShotOutcomeTaken,
		"The screenshot was taken.", providersNow)
}

func TestPreviewShotResultGolden(t *testing.T) {
	testutil.Golden(t, "preview-shot-result", sampleShotResult())
}

// A screenshot check is never silently passed. The two outcomes are the words the wire carries, and a
// skipped check always carries the sentence that says why, so a client never reads an empty shot list
// as a check that passed (docs/library-docs.md).
func TestASkippedShotSaysWhy(t *testing.T) {
	outcomes := protocol.PreviewShotOutcomeValues()
	if len(outcomes) != 2 ||
		outcomes[0] != protocol.PreviewShotOutcomeTaken ||
		outcomes[1] != protocol.PreviewShotOutcomeSkipped {
		t.Fatalf("PreviewShotOutcomeValues = %v, want taken then skipped", outcomes)
	}
	if protocol.PreviewShotOutcome("errored").Valid() {
		t.Error("Valid accepts a shot outcome Marshal never answers with")
	}
	skipped := protocol.NewPreviewShotResult(samplePreview(), protocol.PreviewShotOutcomeSkipped,
		"Marshal found no Chrome or Edge on this machine, so the screenshot check was skipped.", providersNow)
	body, err := json.Marshal(skipped)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"outcome":"skipped"`) {
		t.Errorf("a skipped check encoded as %s, want outcome skipped", body)
	}
	if !strings.Contains(string(body), "found no Chrome or Edge on this machine") {
		t.Errorf("a skipped check must carry the sentence that says why: %s", body)
	}
	if !strings.Contains(string(body), `"serverTime":"2026-09-27T09:30:00.000Z"`) {
		t.Errorf("an answer carries the daemon's time: %s", body)
	}
}

// The three state names are the ones the Preview tab already reads, and it is not redesigned around
// them: a fourth value, or a renamed one, would break the tab silently.
func TestPreviewStatesAreTheThreeTheTabReads(t *testing.T) {
	want := []string{"stopped", "starting", "running"}
	got := protocol.PreviewStateValues()
	if len(got) != len(want) {
		t.Fatalf("PreviewStateValues = %v, want %v", got, want)
	}
	for i, name := range want {
		if string(got[i]) != name {
			t.Fatalf("PreviewStateValues = %v, want %v", got, want)
		}
	}
	if !protocol.PreviewStateStopped.Valid() || protocol.PreviewState("paused").Valid() {
		t.Error("Valid accepts the wrong words")
	}
}

// A preview that is stopped or starting has no address and no start time worth showing: a stopped
// preview that carried an address would send a person to a port nothing is listening on.
func TestAStoppedPreviewCarriesNoAddress(t *testing.T) {
	stopped := protocol.Preview{CardID: sampleCardID, State: protocol.PreviewStateStopped}
	body, err := json.Marshal(protocol.NewPreviewSnapshot(stopped, providersNow))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"startedAt":null`) {
		t.Errorf("a stopped preview encoded as %s, want a null startedAt", body)
	}
	if !strings.Contains(string(body), `"shots":[]`) {
		t.Errorf("a preview with no screenshots encoded as %s, want []", body)
	}
	if !strings.Contains(string(body), `"serverTime":"2026-09-27T09:30:00.000Z"`) {
		t.Errorf("an answer carries the daemon's time: %s", body)
	}
}

// A preview request asks for one of two things, so an unknown value is refused rather than guessed.
func TestAShotIsEitherHalfOfThePair(t *testing.T) {
	got := protocol.PreviewShotKindValues()
	if len(got) != 2 || got[0] != protocol.PreviewShotKindBefore || got[1] != protocol.PreviewShotKindAfter {
		t.Fatalf("PreviewShotKindValues = %v, want before then after", got)
	}
	if protocol.PreviewShotKind("during").Valid() {
		t.Error("Valid accepts a shot that is neither half")
	}
}
