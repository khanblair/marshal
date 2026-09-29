package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestFolderListingGolden(t *testing.T) {
	testutil.Golden(t, "folder-listing", protocol.FolderListing{
		Path: "/Users/ada/code", Parent: "/Users/ada", Home: "/Users/ada", IsGitRepo: false,
		Folders: []protocol.FolderEntry{
			{Name: "marshal", Path: "/Users/ada/code/marshal", IsGitRepo: true},
			{Name: "notes", Path: "/Users/ada/code/notes", IsGitRepo: false},
		},
		Truncated:  false,
		ServerTime: protocol.NewTimestamp(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)),
	})
}
