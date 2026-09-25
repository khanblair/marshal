package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const statusTimeout = 3 * time.Second

func runStatus(args []string, term terminal) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(term.stderr)
	dev := fs.Bool("dev", false, "check the dev daemon")
	port := fs.Int("port", 0, "port of the daemon (default 47800, or 47801 with --dev)")
	if err := fs.Parse(args); err != nil {
		return exitBadInput
	}
	if *port == 0 {
		*port = config.DefaultPort
		if *dev {
			*port = config.DevPort
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()
	health, err := fetchHealth(ctx, *port)
	if err != nil {
		say(term.stderr, "The daemon is not running on port %d.", *port)
		return exitFailed
	}
	say(term.stdout, "The daemon is running on port %d (version %s, %s mode).", *port, health.Version, health.Mode)
	return exitOK
}

// fetchHealth asks the daemon on the local machine for its health.
func fetchHealth(ctx context.Context, port int) (protocol.Health, error) {
	url := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/v1/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return protocol.Health{}, fmt.Errorf("build the request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return protocol.Health{}, fmt.Errorf("ask the daemon: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return protocol.Health{}, errors.New("the daemon did not answer with success")
	}
	var health protocol.Health
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return protocol.Health{}, fmt.Errorf("read the answer: %w", err)
	}
	return health, nil
}
