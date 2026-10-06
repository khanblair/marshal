package store

import (
	"bytes"
	"log/slog"
	"testing"
)

// Migration 0033 gives a schedule its starter, its parts, the chats it goes to, and the quiet flag
// (0033_schedule_templates.sql). These tests prove a schedule that already exists keeps behaving the
// way it did: no starter, no parts, no chats, and not quiet.

func TestMigration0033KeepsEveryExistingScheduleAsItWas(t *testing.T) {
	ctx := testContext(t)
	writer, _ := seedThrough0008(ctx, t)
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if err := migrate(ctx, writer, migrationsThrough(t, 32), log); err != nil {
		t.Fatalf("apply migrations through 0032: %v", err)
	}
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO schedules (id, name, kind, trigger_type) VALUES ('old1', 'Morning brief', 'brief', 'Cron')`); err != nil {
		t.Fatalf("seed a schedule made before the migration: %v", err)
	}
	if err := migrate(ctx, writer, embeddedFiles(t), log); err != nil {
		t.Fatalf("apply the remaining migrations: %v", err)
	}
	var template, sections, deliver string
	var quiet int
	err := writer.QueryRowContext(ctx,
		`SELECT template, sections_json, deliver_json, quiet_when_empty FROM schedules WHERE id = 'old1'`).
		Scan(&template, &sections, &deliver, &quiet)
	if err != nil {
		t.Fatalf("read the schedule back: %v", err)
	}
	if template != "" || sections != "[]" || deliver != "[]" || quiet != 0 {
		t.Fatalf("the old schedule is %q %q %q %d after the migration, want no starter, no parts, no chats, not quiet",
			template, sections, deliver, quiet)
	}
}

func TestTheQuietFlagTakesOnlyZeroOrOne(t *testing.T) {
	ctx := testContext(t)
	writer, _ := seedThrough0008(ctx, t)
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if err := migrate(ctx, writer, embeddedFiles(t), log); err != nil {
		t.Fatalf("apply the migrations: %v", err)
	}
	if _, err := writer.ExecContext(ctx,
		`INSERT INTO schedules (id, name, kind, trigger_type, quiet_when_empty) VALUES ('s1', 'A', 'brief', 'Cron', 2)`); err == nil {
		t.Fatal("a schedule with the quiet flag set to 2 was stored")
	}
}
