package harness_test

import (
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The stuck detector (B5.3, build-plan 5.4): the pure decision half. It is fed one signal at a time
// and reports when the same one appears a set number of times in a row. The wiring that feeds it an
// agent's events is tested where the sessions live.

// errorSignal is a failed tool call: its key fingerprints the failure and the label is the word a
// person reads. The wording of the sentence is the prototype's own ("the same type error appeared
// three times in a row in columns.tsx").
func errorSignal(fingerprint, label, where string) harness.Signal {
	return harness.Signal{Kind: harness.SignalError, Key: fingerprint, Label: label, Where: where}
}

func editSignal(path string) harness.Signal {
	return harness.Signal{Kind: harness.SignalEdit, Key: path, Where: path}
}

// The same failure three times in a row is a loop, and the sentence names what repeated and where,
// in the words the caller described the failure with.
func TestTheSameFailureThreeTimesInARowIsALoop(t *testing.T) {
	detector := harness.NewStuckDetector()
	signal := errorSignal("TS2322: Type 'string' is not assignable to type 'number'", "type error", "columns.tsx")

	if _, stuck := detector.Observe(signal); stuck {
		t.Fatal("the detector called the card stuck after one failure")
	}
	if _, stuck := detector.Observe(signal); stuck {
		t.Fatal("the detector called the card stuck after two failures")
	}
	reason, stuck := detector.Observe(signal)
	if !stuck {
		t.Fatal("the detector did not call the card stuck after the same failure three times")
	}
	if reason.Kind != protocol.NeedsReasonKindStuck {
		t.Errorf("kind = %q, want %q", reason.Kind, protocol.NeedsReasonKindStuck)
	}
	want := "The stuck detector paused this card. The same type error appeared 3 times in a row in columns.tsx."
	if reason.Text != want {
		t.Errorf("text = %q, want %q", reason.Text, want)
	}
}

// Two different failures alternating are two runs of one, not a loop: a card fighting two separate
// problems is not the same as a card going round in circles.
func TestAlternatingFailuresAreNotALoop(t *testing.T) {
	detector := harness.NewStuckDetector()
	first := errorSignal("error one", "type error", "a.ts")
	second := errorSignal("error two", "test failure", "b.ts")
	for i := 0; i < 6; i++ {
		for _, signal := range []harness.Signal{first, second} {
			if _, stuck := detector.Observe(signal); stuck {
				t.Fatalf("an alternating pair was called a loop at step %d", i)
			}
		}
	}
}

// A signal with no key cannot be fingerprinted, so it is not a loop and does not break the run of
// the one that can: a failure the agent did not describe well enough is ignored, not counted.
func TestASignalWithNoKeyIsIgnoredAndLeavesTheRunAlone(t *testing.T) {
	detector := harness.NewStuckDetector()
	signal := errorSignal("the same failure", "type error", "a.ts")
	detector.Observe(signal)
	detector.Observe(signal)
	if _, stuck := detector.Observe(harness.Signal{Kind: harness.SignalError}); stuck {
		t.Fatal("an unfingerprinted failure was counted toward the loop")
	}
	if _, stuck := detector.Observe(harness.Signal{Kind: harness.SignalError, Key: "   "}); stuck {
		t.Fatal("a blank failure was counted toward the loop")
	}
	reason, stuck := detector.Observe(signal)
	if !stuck {
		t.Fatal("a blank signal broke the run of the failure that repeated")
	}
	if !strings.Contains(reason.Text, "3 times in a row") {
		t.Errorf("text = %q, want it to count only the three real signals", reason.Text)
	}
}

// An edit loop is the same file rewritten again and again, and the sentence names the file.
func TestTheSameEditThreeTimesInARowIsALoop(t *testing.T) {
	detector := harness.NewStuckDetector()
	signal := editSignal("src/columns.tsx")
	detector.Observe(signal)
	detector.Observe(signal)
	reason, stuck := detector.Observe(signal)
	if !stuck {
		t.Fatal("the detector did not catch the same file edited three times in a row")
	}
	if reason.Kind != protocol.NeedsReasonKindStuck {
		t.Errorf("kind = %q, want %q", reason.Kind, protocol.NeedsReasonKindStuck)
	}
	if !strings.Contains(reason.Text, "src/columns.tsx") {
		t.Errorf("text = %q, want it to name the file", reason.Text)
	}
}

// A different signal starts a new run: two failures then three edits is the edit loop, and the card
// is stopped for the run that reached the threshold.
func TestADifferentSignalStartsANewRun(t *testing.T) {
	detector := harness.NewStuckDetector()
	failure := errorSignal("an error", "type error", "a.ts")
	detector.Observe(failure)
	detector.Observe(failure)
	edit := editSignal("a.ts")
	if _, stuck := detector.Observe(edit); stuck {
		t.Fatal("a single edit after two failures was called a loop")
	}
	detector.Observe(edit)
	reason, stuck := detector.Observe(edit)
	if !stuck {
		t.Fatal("the edit run did not reach the threshold")
	}
	if !strings.Contains(reason.Text, "edited 3 times") {
		t.Errorf("text = %q, want the edit loop named", reason.Text)
	}
}

// A failure with no label still reads as a sentence: the detector's own word for the kind is used,
// and a failure with no place leaves that part out rather than saying "in .".
func TestAFailureWithNoLabelOrPlaceStillReads(t *testing.T) {
	detector := harness.NewStuckDetector()
	signal := errorSignal("some failure", "", "")
	detector.Observe(signal)
	detector.Observe(signal)
	reason, stuck := detector.Observe(signal)
	if !stuck {
		t.Fatal("the failure loop was not caught")
	}
	want := "The stuck detector paused this card. The same error appeared 3 times in a row."
	if reason.Text != want {
		t.Errorf("text = %q, want %q", reason.Text, want)
	}
}

// The threshold is configurable, and one below two is raised to two: a detector told to catch a
// single signal would catch nothing but noise, since a key with no repeat is not a loop.
func TestTheThresholdIsRespectedAndNeverBelowTwo(t *testing.T) {
	detector := harness.NewStuckDetectorN(2)
	signal := errorSignal("an error", "type error", "a.ts")
	detector.Observe(signal)
	if _, stuck := detector.Observe(signal); !stuck {
		t.Error("a threshold of two did not catch the second signal")
	}

	raised := harness.NewStuckDetectorN(0)
	raised.Observe(signal)
	if _, stuck := raised.Observe(signal); !stuck {
		t.Error("a threshold below two was not raised to two")
	}
}
