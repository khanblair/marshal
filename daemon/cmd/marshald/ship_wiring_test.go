package main

import (
	"context"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

type fakeCI struct {
	result protocol.LocalCIResult
	err    error
}

func (f fakeCI) Run(context.Context, string, string) (protocol.LocalCIResult, error) {
	return f.result, f.err
}

func ciWith(steps ...protocol.LocalCIStep) fakeCI {
	return fakeCI{result: protocol.LocalCIResult{Workflows: []protocol.LocalCIWorkflow{{Name: "ci", Steps: steps}}}}
}

func TestLocalChecksPassWhenEveryStepThatRanPassed(t *testing.T) {
	out, err := localChecks{ci: ciWith(
		protocol.LocalCIStep{Job: "test", Name: "go test", Status: protocol.LocalCIStatusPassed},
		protocol.LocalCIStep{Job: "test", Name: "deploy", Status: protocol.LocalCIStatusUnsupported},
	)}.CheckChanges(context.Background(), "c1")
	if err != nil || !out.Ran || !out.Passed {
		t.Errorf("outcome = %+v, %v, want ran and passed", out, err)
	}
}

func TestLocalChecksSayWhatFailedAndWhatItPrinted(t *testing.T) {
	out, err := localChecks{ci: ciWith(
		protocol.LocalCIStep{Job: "test", Name: "go test", Status: protocol.LocalCIStatusFailed, Output: "FAIL TestRetry"},
	)}.CheckChanges(context.Background(), "c1")
	if err != nil || !out.Ran || out.Passed {
		t.Fatalf("outcome = %+v, %v, want ran and failed", out, err)
	}
	if !strings.Contains(out.Summary, "go test failed") || !strings.Contains(out.Summary, "FAIL TestRetry") {
		t.Errorf("summary = %q, want the step and its output", out.Summary)
	}
}

func TestLocalChecksWithNothingToRunAreNotAFailure(t *testing.T) {
	out, err := localChecks{ci: ciWith(
		protocol.LocalCIStep{Job: "deploy", Name: "publish", Status: protocol.LocalCIStatusUnsupported},
	)}.CheckChanges(context.Background(), "c1")
	if err != nil || out.Ran {
		t.Errorf("outcome = %+v, %v, want nothing ran", out, err)
	}
	if out, err := (localChecks{ci: fakeCI{}}).CheckChanges(context.Background(), "c1"); err != nil || out.Ran {
		t.Errorf("a project with no workflows = %+v, %v, want nothing ran", out, err)
	}
}
