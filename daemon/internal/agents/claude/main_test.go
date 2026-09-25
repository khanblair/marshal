package claude

import (
	"fmt"
	"os"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// The fake Claude Code is this test binary started again, speaking the stream-json wire
	// format that Claude Code 2.1.282 uses. Its behavior comes from the environment (see
	// fake_test.go). No test in this package drives the real claude binary, per the hard safety
	// rule for this adapter.
	if flags, ok := os.LookupEnv(fakeEnv); ok {
		runFakeClaude(flags)
		os.Exit(0)
	}
	code := m.Run()
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}
