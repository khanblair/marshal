package projects

import (
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// The messages people read. They are plain sentences: they say what happened and, when there is
// something to do, what to do next. They never hold a path, so the API can send them as they are.
const (
	messageEmptyName         = "Project names can't be empty. The old name is kept."
	messageNotARepo          = "That folder is not a Git repository. Choose the top folder of a repository."
	messageNoFolder          = "That folder does not exist. Check the path and try again."
	messageNotAFolder        = "That is not a folder. Choose the top folder of a repository."
	messageRelativePath      = "Enter the full path of the folder, starting from the top of the disk or with ~."
	messageAlreadyAdded      = "That repository is already a project in Marshal."
	messageDestInsideAnother = "Choose a new folder for the clone. It cannot be inside a project that Marshal already manages."
	messageNeedsGit          = "Marshal needs Git 2.38 or newer, and could not find it on this machine."
	messageNoRepoFolder      = "Marshal cannot reach the repository folder. Check that it is still there."
)

func notFoundProject(id string) *protocol.Error {
	return protocol.NotFound("project").With("id", id)
}

func notFoundCard(id string) *protocol.Error {
	return protocol.NotFound("card").With("id", id)
}

// notFound turns the store's "no row" into the not found answer for a kind of thing, and leaves
// every other error as it is.
func notFound(err error, missing *protocol.Error) error {
	if store.IsNotFound(err) {
		return missing
	}
	return err
}
