package integrations

// The Obsidian vault connection's half of the connection tests (docs/architecture.md section 18,
// docs/backend-checklist.md B7.4, build-plan task 7.7). Section 18's table gives it one line:
// "Obsidian vault | Folder exists and is writable".
//
// It is the one connection in that table that is not a service. Marshal keeps a project's memory as
// markdown in a vault folder of its own (`<data>/vault`, section 12), so there is nothing for a
// person to connect, no key to store, and nothing to dial: the test asks the file system where the
// vault is, whether it is there, and whether Marshal may write in it. That is also why its row's
// status comes from the vault itself rather than from a saved setting (integrations.go).
//
// The writability check reads the folder's own permission bits rather than leaving a probe file
// behind, because section 18 keeps a connection test read-only. What that proves is exactly what the
// sentence says: the folder's own permissions let Marshal write in it. A folder whose permissions
// say yes but whose ownership says no is not something a checkpoint in a settings screen can settle.

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The names of the checks the Obsidian test reports. They are the row labels a screen shows, so they
// are words rather than ids, and - like every connection's - the summary is named CheckSummary so a
// row reads back what the test found without the list route knowing what this test asks.
const (
	// CheckVaultFolder is the check that the vault folder is where Marshal keeps memory.
	CheckVaultFolder = "Vault folder"
	// CheckVaultWritable is the check that the folder's permissions let Marshal write in it.
	CheckVaultWritable = "Vault writable"
)

// writeBit is the owner's write permission in a file mode: the `0200` of a mode like `0700`.
const writeBit = 0o200

// testObsidian is the real Obsidian test: where the vault is, whether it is there, and whether
// Marshal may write in it. Nothing about a missing folder is an error - a daemon nobody has saved a
// note with yet has no vault folder, and the first save makes it - so "not there yet" is a warning
// carrying the sentence that says so.
func (s *Service) testObsidian(ctx context.Context, info Info) (protocol.TestResult, error) {
	if err := ctx.Err(); err != nil {
		return protocol.TestResult{}, err
	}
	if s.vault == "" {
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name:    CheckSummary,
			State:   protocol.CheckStateWarning,
			Message: "Marshal does not know where its vault folder is, so it cannot test Obsidian.",
			Fix:     "Check the daemon's data folder, then test again.",
		}}, s.now()), nil
	}
	folder := vaultFolderCheck(s.vault)
	checks := []protocol.TestCheck{folder, vaultWritableCheck(s.vault, folder.State)}
	checks = append([]protocol.TestCheck{summaryCheck(checks,
		"Marshal's vault is ready to open in Obsidian.",
		"Marshal's vault is ready, with something to check.")}, checks...)
	return protocol.NewTestResult(info.ID, checks, s.now()), nil
}

// vaultFolderCheck is whether the vault folder is where it should be. A folder that is not there yet
// is a warning rather than a failure, because Marshal makes it when the first note is saved and a
// fresh daemon has simply not saved one.
func vaultFolderCheck(path string) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckVaultFolder}
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		check.State = protocol.CheckStateWarning
		check.Message = fmt.Sprintf("The vault folder is not there yet (%s).", path)
		check.Fix = "Save a note from any card's Notes tab, and Marshal makes the folder."
	case err != nil:
		check.State = protocol.CheckStateFailed
		check.Message = fmt.Sprintf("Marshal could not look at the vault folder: %s.", err)
		check.Fix = "Check that the vault folder may be read, then test again."
	case !info.IsDir():
		check.State = protocol.CheckStateFailed
		check.Message = fmt.Sprintf("%s is a file, and Marshal needs a folder there.", path)
		check.Fix = "Move the file out of the way, then test again."
	default:
		check.State = protocol.CheckStatePassed
		check.Message = fmt.Sprintf("The vault folder is at %s.", path)
	}
	return check
}

// vaultWritableCheck is whether the folder's own permissions let Marshal write in it. It answers a
// warning when the folder is not there, since there is nothing yet whose permissions could be read,
// so the test never claims to have proved something it could not look at.
func vaultWritableCheck(path string, folder protocol.CheckState) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckVaultWritable}
	if folder != protocol.CheckStatePassed {
		check.State = protocol.CheckStateWarning
		check.Message = "Marshal can check whether it may write in the vault once the folder is there."
		return check
	}
	info, err := os.Stat(path)
	if err != nil {
		check.State = protocol.CheckStateFailed
		check.Message = fmt.Sprintf("Marshal could not look at the vault folder: %s.", err)
		check.Fix = "Check that the vault folder may be read, then test again."
		return check
	}
	if info.Mode().Perm()&writeBit == 0 {
		check.State = protocol.CheckStateFailed
		check.Message = "The vault folder is read-only, so Marshal cannot save notes in it."
		check.Fix = fmt.Sprintf("Give your user write access to %s, then test again.", path)
		return check
	}
	check.State = protocol.CheckStatePassed
	check.Message = "The vault folder's permissions let Marshal write in it."
	return check
}
