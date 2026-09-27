package roles

import (
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// The messages people read. They are plain sentences: they say what happened and, when there is
// something to do, what to do next. They never hold a path.
const (
	messageNameInUse = "A role called \"%s\" already exists. Choose another name."
)

// notFoundRole is the answer when a role is not there.
func notFoundRole(name string) *protocol.Error {
	return protocol.NotFound("role").With("name", name)
}

// conflictName is the answer when a role is given a name another role already has.
func conflictName(name string) *protocol.Error {
	return protocol.Conflict(fmt.Sprintf(messageNameInUse, name)).With("name", name)
}

// notFoundRoleFrom turns the store's "no row" into a missing role, and leaves every other error as
// it is.
func notFoundRoleFrom(err error, name string) error {
	if store.IsNotFound(err) {
		return notFoundRole(name)
	}
	return err
}

// notFoundProjectFrom turns the store's "no row" into a missing project, and leaves every other
// error as it is.
func notFoundProjectFrom(err error, id string) error {
	if store.IsNotFound(err) {
		return protocol.NotFound("project").With("id", id)
	}
	return err
}
