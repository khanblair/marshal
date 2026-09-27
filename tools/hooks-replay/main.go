// Command hooks-replay sends a recorded webhook to the dev daemon, so webhook handling can be
// developed without any internet setup. Recordings live in daemon/testdata/hooks.
//
//	hooks-replay [-url http://127.0.0.1:47801] [-dir path] [-secret s] <provider> <name>
//
// With -secret, the recorded body is signed with it, so a fixture saved without a signature (as a
// recording made before a secret existed) can still be replayed against a daemon holding that
// secret. The secret is the same one saved in Settings, and it is never written anywhere.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	defaultURL     = "http://127.0.0.1:47801"
	defaultDir     = "../../daemon/testdata/hooks"
	requestTimeout = 10 * time.Second
	exitOK         = 0
	exitFailed     = 1
	exitBadInput   = 2
	wantArgs       = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("hooks-replay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	url := fs.String("url", defaultURL, "address of the dev daemon")
	dir := fs.String("dir", defaultDir, "folder with the recordings")
	secret := fs.String("secret", "", "webhook secret to sign the body with, empty to send it as recorded")
	if err := fs.Parse(args); err != nil {
		return exitBadInput
	}
	if fs.NArg() != wantArgs {
		say(stderr, "Usage: hooks-replay [-url address] [-dir folder] [-secret secret] <provider> <name>")
		return exitBadInput
	}
	provider, name := fs.Arg(0), fs.Arg(1)
	rec, err := loadRecording(*dir, provider, name)
	if err != nil {
		say(stderr, "%v", err)
		return exitBadInput
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	status, err := send(ctx, http.DefaultClient, target{baseURL: *url, provider: provider}, rec, *secret)
	if err != nil {
		say(stderr, "%v", err)
		return exitFailed
	}
	say(stdout, "Sent %s/%s to %s/hooks/%s: %s", provider, name, *url, provider, status)
	return exitOK
}
