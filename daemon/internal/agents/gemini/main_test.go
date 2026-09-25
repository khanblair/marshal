package gemini

import (
	"fmt"
	"os"
	"testing"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestMain(m *testing.M) {
	// The fake Gemini CLI is this test binary started again, speaking ACP the way Gemini CLI 0.35.1
	// does. Its behaviors come in the environment (see fake_test.go).
	if flags := os.Getenv(fakeEnv); flags != "" {
		runFakeGemini(flags)
		os.Exit(0)
	}
	code := m.Run()
	testutil.CleanStubAgent()
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}
