package store

import (
	"bufio"
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
)

const (
	migrationsDir = "migrations"
	// downMarker starts the section of a goose file that undoes it, once the leading dashes and
	// spaces are gone and the text is in lower case. We never undo a migration.
	downMarker = "+goose down"
	comment    = "--"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// embeddedMigrations is the folder of numbered SQL files that ships inside the daemon.
func embeddedMigrations() (fs.FS, error) {
	files, err := fs.Sub(migrationFiles, migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("find the migrations: %w", err)
	}
	return files, nil
}

// migrate applies every migration in files that has not run yet. It runs on the writer connection
// before any reader exists. Goose keeps its own version table in the same database.
func migrate(ctx context.Context, writer *sql.DB, files fs.FS, log *slog.Logger) error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, writer, files,
		goose.WithSlog(log), goose.WithDisableGlobalRegistry(true))
	if err != nil {
		return fmt.Errorf("prepare the migrations: %w", err)
	}
	results, err := provider.Up(ctx)
	for _, result := range results {
		log.Info("applied a database migration",
			"version", result.Source.Version,
			"duration_ms", result.Duration.Round(time.Millisecond).Milliseconds())
	}
	if err != nil {
		return fmt.Errorf("apply the migrations: %w", err)
	}
	return nil
}

// checkForwardOnly returns an error naming the first file in files that has a Down section. The
// test suite runs it on the embedded migrations, so a migration that can be undone fails the build.
func checkForwardOnly(files fs.FS) error {
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return fmt.Errorf("list the migrations: %w", err)
	}
	for _, name := range names {
		found, err := hasDownSection(files, name)
		if err != nil {
			return err
		}
		if found {
			return fmt.Errorf("migration %s has a Down section: migrations are forward only", name)
		}
	}
	return nil
}

func hasDownSection(files fs.FS, name string) (bool, error) {
	file, err := files.Open(name)
	if err != nil {
		return false, fmt.Errorf("read migration %s: %w", name, err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if rest, isComment := strings.CutPrefix(line, comment); isComment &&
			strings.HasPrefix(strings.TrimSpace(rest), downMarker) {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("read migration %s: %w", name, err)
	}
	return false, nil
}
