package testutil

import (
	"context"
	"os"
	"sync"
	"testing"
)

// approvalCLIBuild is the one build of the approval CLI fixture that a test process makes.
var approvalCLIBuild struct { //nolint:gochecknoglobals // one build per test process, shared by every test
	once sync.Once
	dir  string
	path string
	err  error
}

// ApprovalCLI builds the standalone CLI fixture used by API approval tests once per process.
// A package using it calls CleanApprovalCLI from TestMain.
func ApprovalCLI(t testing.TB) string {
	t.Helper()
	approvalCLIBuild.once.Do(func() {
		approvalCLIBuild.dir, approvalCLIBuild.path, approvalCLIBuild.err = buildApprovalCLI(context.Background(), moduleDir(t))
	})
	if approvalCLIBuild.err != nil {
		t.Fatalf("build the approval CLI fixture: %v", approvalCLIBuild.err)
	}
	return approvalCLIBuild.path
}

// CleanApprovalCLI removes the build that ApprovalCLI made. It does nothing when no build was made.
func CleanApprovalCLI() {
	if approvalCLIBuild.dir != "" {
		_ = os.RemoveAll(approvalCLIBuild.dir)
	}
}

func buildApprovalCLI(ctx context.Context, module string) (dir, path string, err error) {
	return buildGoBinary(ctx, module, "./internal/api/testdata/approvalcli", "marshal-approval-cli-", "approval-cli")
}
