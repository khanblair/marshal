package ci_test

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestTheRealModeKeepsThePushOffTheIntegrationBranch(t *testing.T) {
	f := newFixture(t)
	f.projects.projects[0].IntegrationBranch = "development"
	f.simulate(t, protocol.SimulateModeReal)
	if len(f.git.pushed) != 1 || f.git.pushed[0].main != "development" {
		t.Fatalf("pushes = %+v, want one that stays off development", f.git.pushed)
	}
}
