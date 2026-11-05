package harness_test

import (
	"fmt"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/harness"
)

func TestDifferentEditsToOneFileAreProgress(t *testing.T) {
	d := harness.NewStuckDetector()
	for i := range 100 {
		_, stuck := d.Observe(harness.Signal{Kind: harness.SignalEdit, Key: "main.go", Where: "main.go", Fingerprint: fmt.Sprint(i)})
		if stuck {
			t.Fatalf("distinct edit %d called a loop", i)
		}
	}
}

func TestRepeatedAndBackAndForthEditsAreStillLoops(t *testing.T) {
	for _, fingerprints := range [][]string{{"a", "a", "a"}, {"a", "b", "a"}} {
		d := harness.NewStuckDetector()
		for i, fingerprint := range fingerprints {
			_, stuck := d.Observe(harness.Signal{Kind: harness.SignalEdit, Key: "main.go", Where: "main.go", Fingerprint: fingerprint})
			if stuck != (i == 2) {
				t.Fatalf("sequence %v, edit %d: stuck=%v", fingerprints, i, stuck)
			}
		}
	}
}

func TestMissingEditDataKeepsTheConservativeGuard(t *testing.T) {
	d := harness.NewStuckDetector()
	for i, fingerprint := range []string{"a", "", "b"} {
		_, stuck := d.Observe(harness.Signal{Kind: harness.SignalEdit, Key: "main.go", Fingerprint: fingerprint})
		if stuck != (i == 2) {
			t.Fatalf("unknown edit data bypassed the guard at %d", i)
		}
	}
}

func TestKnownEditsRespectTheTwoRepeatThreshold(t *testing.T) {
	d := harness.NewStuckDetectorN(2)
	for i, fingerprint := range []string{"a", "b", "b"} {
		_, stuck := d.Observe(harness.Signal{Kind: harness.SignalEdit, Key: "main.go", Fingerprint: fingerprint})
		if stuck != (i == 2) {
			t.Fatalf("threshold ignored at %d", i)
		}
	}
}
