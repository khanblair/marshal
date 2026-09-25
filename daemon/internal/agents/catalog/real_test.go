package catalog

import (
	"os"
	"testing"
)

// TestRealMachine looks at the machine the test runs on, prints what it finds, and starts nothing
// but "<program> --version". It is off unless MARSHAL_REAL_AGENTS=1, because what it finds depends
// on the machine. Run it with -v to see the output:
//
//	MARSHAL_REAL_AGENTS=1 go test -run TestRealMachine -v ./internal/agents/catalog/
func TestRealMachine(t *testing.T) {
	if os.Getenv("MARSHAL_REAL_AGENTS") != "1" {
		t.Skip("set MARSHAL_REAL_AGENTS=1 to look at the agents that are installed on this machine")
	}
	c := New(Options{})
	detected, err := c.Detect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range detected {
		t.Logf("%-7s status=%-9s version=%-8s startable=%-5v path=%s warning=%q",
			d.Kind, d.Status, d.Version, d.Startable, d.Path, d.Warning)
	}
}
