// Command marshald is the Marshal daemon. It owns all state and does all the work, and keeps
// running when the app is closed.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/buildinfo"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/store"
)

const (
	dataDirMode  = 0o700
	databaseFile = "marshal.db"
	exitOK       = 0
	exitFailed   = 1
	exitBadInput = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run starts the daemon and returns the process exit code. It is separate from main so tests
// can call it.
func run(args []string, stdout, stderr io.Writer) int {
	env, err := platform.CurrentEnv()
	if err != nil {
		say(stderr, "%v", err)
		return exitFailed
	}
	settings, err := config.Load(args, env, stderr)
	switch {
	case errors.Is(err, config.ErrHelp):
		return exitOK
	case err != nil:
		say(stderr, "%v", err)
		return exitBadInput
	}
	if settings.ShowVersion {
		say(stdout, "%s", buildinfo.Version)
		return exitOK
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve(ctx, settings, slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: settings.LogLevel}))); err != nil {
		say(stderr, "%v", err)
		return exitFailed
	}
	return exitOK
}

// serve opens what the daemon owns, serves until the context ends, and closes it in the reverse
// order: the server stops accepting and closes its event streams first (Run does that), then the
// event bus closes, then the database.
func serve(ctx context.Context, settings config.Settings, log *slog.Logger) (err error) {
	if err := os.MkdirAll(settings.DataDir, dataDirMode); err != nil {
		return fmt.Errorf("make the data folder %s: %w", settings.DataDir, err)
	}
	log.Info("starting", "version", buildinfo.Version, "mode", settings.Mode, "data_dir", settings.DataDir)
	st, err := store.Open(ctx, filepath.Join(settings.DataDir, databaseFile), store.WithLogger(log))
	if err != nil {
		return fmt.Errorf("open the database: %w", err)
	}
	defer func() {
		if closeErr := st.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close the database: %w", closeErr))
		}
	}()
	bus, err := events.New()
	if err != nil {
		return fmt.Errorf("start the event bus: %w", err)
	}
	defer bus.Close()
	dev, err := api.EnsureAccounts(ctx, st, api.AccountsConfig{
		DataDir: settings.DataDir, Dev: settings.Dev(), Now: time.Now, Log: log,
	})
	if err != nil {
		return err
	}
	return api.New(settings, log, time.Now, api.Deps{Store: st, Bus: bus, Dev: dev}).Run(ctx)
}
