package preview

// The real dev-server runner: the project's dev command as a child process, through the same shell
// and the same filtered environment every other command Marshal runs uses (internal/proc refuses to
// pass on the daemon's own environment, so a dev server cannot read the daemon's secrets).

import (
	"context"
	"io"
	"strconv"

	"github.com/khanblair/marshal/daemon/internal/proc"
)

// CommandRunner starts a dev server as a child process. The port Marshal picked is passed as PORT,
// which is what nearly every dev server reads, so a project's dev command does not have to know about
// Marshal; HOST is set so a server that would listen on every interface listens only on this machine.
type CommandRunner struct{}

// Start runs the command and hands back the process. Its output is drained, so a chatty dev server
// cannot block on a full pipe and hang with no explanation.
func (CommandRunner) Start(ctx context.Context, req DevRequest) (Process, error) {
	child, err := proc.Start(ctx, proc.Spec{
		Path: "/bin/sh",
		Args: []string{"-e", "-c", req.Command},
		Dir:  req.Dir,
		Env:  []string{"HOST=127.0.0.1", "PORT=" + strconv.Itoa(req.Port)},
	})
	if err != nil {
		return nil, err
	}
	go func() { _, _ = io.Copy(io.Discard, child.Stdout) }()
	return child, nil
}
