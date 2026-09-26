package api_test

import (
	"fmt"
	"os"
	"testing"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// TestMain runs the suite, then removes the stub agent that the tests built (testutil.StubAgent
// builds it once for the whole process), and only then checks for goroutines that were left
// running: goleak.VerifyTestMain exits the process itself, so nothing could follow it.
func TestMain(m *testing.M) {
	code := m.Run()
	testutil.CleanStubAgent()
	testutil.CleanTerminalHelper()
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}
