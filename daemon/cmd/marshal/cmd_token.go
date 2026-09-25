package main

import (
	"errors"
	"flag"
	"os"

	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
)

// runToken says where the token file of the daemon is, and with --show prints the token in it. The
// token lets a client act as the owner, so it is never printed unless the person asks for it, and
// never in the answer to any other command.
func runToken(env platform.Env, args []string, term terminal) int {
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	fs.SetOutput(term.stderr)
	dev := fs.Bool("dev", false, "use the dev daemon's token file")
	show := fs.Bool("show", false, "print the token itself, not where it is")
	if err := fs.Parse(args); err != nil {
		return exitBadInput
	}
	var settingsArgs []string
	if *dev {
		settingsArgs = []string{"--dev"}
	}
	settings, err := config.Load(settingsArgs, env, term.stderr)
	if err != nil {
		say(term.stderr, "%v", err)
		return exitFailed
	}
	path := platform.TokenPath(settings.DataDir, settings.Mode)
	token, err := platform.ReadTokenFile(path)
	if errors.Is(err, os.ErrNotExist) {
		say(term.stderr, "There is no token at %s yet. Start the daemon once to make it.", path)
		return exitFailed
	}
	if err != nil {
		say(term.stderr, "%v", err)
		return exitFailed
	}
	if *show {
		say(term.stdout, "%s", token)
		return exitOK
	}
	say(term.stdout, "%s", path)
	return exitOK
}
