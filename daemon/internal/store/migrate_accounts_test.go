package store

import (
	"bytes"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"
)

// Migration 0010 (docs/backend-checklist.md B2.2, B2.5, and B2.13): the avatar and progress columns,
// a project's saved views, and a person's preferences. These tests are about the rules the tables
// keep for themselves, because the two modules that use them are tested through Go and never write
// a row the tables would refuse.

// execAll runs statements on the writer, and fails the test at the first one that is refused.
func execAll(t *testing.T, s *Store, statements ...string) {
	t.Helper()
	for _, statement := range statements {
		if _, err := s.writer.ExecContext(testContext(t), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

// wantRefused runs a statement that the database must refuse, and says why in the failure.
func wantRefused(t *testing.T, s *Store, why, statement string) {
	t.Helper()
	if _, err := s.writer.ExecContext(testContext(t), statement); err == nil {
		t.Errorf("the database accepted %s: %s", why, statement)
	}
}

func countRows(t *testing.T, s *Store, query string) int {
	t.Helper()
	var n int
	if err := s.reader.QueryRowContext(testContext(t), query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// The three new tables are STRICT, like every table, so a value of the wrong type is refused.
func TestMigration0010TablesAreStrict(t *testing.T) {
	s := openTemp(t)
	for _, table := range []string{"saved_views", "user_preferences", "project_preferences"} {
		var strict int
		row := s.reader.QueryRowContext(testContext(t), "SELECT strict FROM pragma_table_list WHERE name = ?", table)
		if err := row.Scan(&strict); err != nil || strict != 1 {
			t.Errorf("%s: strict = %d, %v; want a STRICT table", table, strict, err)
		}
	}
}

const (
	insertProject = `INSERT INTO projects (id, name, repo_path, created_at, updated_at) VALUES ('api', 'api', '/tmp/api', 1, 1)`
	insertUser    = `INSERT INTO users (id, name, created_at, updated_at) VALUES ('u1', 'Ada', 1, 1)`
	insertView    = `INSERT INTO saved_views (id, project_id, name, created_at, updated_at) VALUES ('v1', 'api', 'Needs me', 1, 1)`
)

// A view has a name that its project uses once, defaults to no filters and no swimlane, and goes with
// its project.
func TestMigration0010SavedViewsBelongToTheirProject(t *testing.T) {
	s := openTemp(t)
	execAll(t, s, insertProject, insertView)
	var filters, swimlane string
	row := s.reader.QueryRowContext(testContext(t), "SELECT filters_json, swimlane FROM saved_views WHERE id = 'v1'")
	if err := row.Scan(&filters, &swimlane); err != nil || filters != "[]" || swimlane != "none" {
		t.Errorf("defaults = %q, %q, %v; want [] and none", filters, swimlane, err)
	}
	wantRefused(t, s, "a second view with the same name in one project",
		`INSERT INTO saved_views (id, project_id, name, created_at, updated_at) VALUES ('v2', 'api', 'Needs me', 1, 1)`)
	wantRefused(t, s, "a view of a project that is not there",
		`INSERT INTO saved_views (id, project_id, name, created_at, updated_at) VALUES ('v3', 'nope', 'x', 1, 1)`)
	wantRefused(t, s, "a time written as text",
		`INSERT INTO saved_views (id, project_id, name, created_at, updated_at) VALUES ('v4', 'api', 'y', 'yesterday', 1)`)
	execAll(t, s,
		`INSERT INTO projects (id, name, repo_path, created_at, updated_at) VALUES ('web', 'web', '/tmp/web', 1, 1)`,
		`INSERT INTO saved_views (id, project_id, name, created_at, updated_at) VALUES ('v5', 'web', 'Needs me', 1, 1)`)
	execAll(t, s, `DELETE FROM projects WHERE id = 'api'`)
	if n := countRows(t, s, "SELECT COUNT(*) FROM saved_views"); n != 1 {
		t.Errorf("%d saved views are left after their project went, want only the other project's", n)
	}
}

// A person's project preferences go with their project, and a saved view that is deleted leaves no
// view in use rather than an id that points at nothing.
func TestMigration0010PreferencesFollowTheirProjectAndSavedView(t *testing.T) {
	s := openTemp(t)
	execAll(t, s, insertProject, insertUser, insertView,
		`INSERT INTO project_preferences (user_id, project_id, saved_view_id, updated_at) VALUES ('u1', 'api', 'v1', 1)`)
	var view, swimlane string
	var showAllDone int
	row := s.reader.QueryRowContext(testContext(t),
		"SELECT last_view, swimlane, show_all_done FROM project_preferences WHERE user_id = 'u1'")
	if err := row.Scan(&view, &swimlane, &showAllDone); err != nil || view != "board" || swimlane != "none" || showAllDone != 0 {
		t.Errorf("defaults = %q, %q, %d, %v; want board, none, 0", view, swimlane, showAllDone, err)
	}

	execAll(t, s, `DELETE FROM saved_views WHERE id = 'v1'`)
	var linked sql.NullString
	if err := s.reader.QueryRowContext(testContext(t), "SELECT saved_view_id FROM project_preferences").Scan(&linked); err != nil || linked.Valid {
		t.Errorf("the saved view in use = %+v, %v; want none after its view was deleted", linked, err)
	}
	execAll(t, s, `DELETE FROM projects WHERE id = 'api'`)
	if n := countRows(t, s, "SELECT COUNT(*) FROM project_preferences"); n != 0 {
		t.Errorf("%d project preferences are left after their project went", n)
	}
	wantRefused(t, s, "preferences of a project that is not there",
		`INSERT INTO project_preferences (user_id, project_id, updated_at) VALUES ('u1', 'nope', 1)`)
	execAll(t, s, insertProject)
	wantRefused(t, s, "preferences of a person who is not there",
		`INSERT INTO project_preferences (user_id, project_id, updated_at) VALUES ('nobody', 'api', 1)`)
	wantRefused(t, s, "a flag that is not 0 or 1",
		`INSERT INTO project_preferences (user_id, project_id, show_all_done, updated_at) VALUES ('u1', 'api', 2, 1)`)
	wantRefused(t, s, "a saved view that is not there",
		`INSERT INTO project_preferences (user_id, project_id, saved_view_id, updated_at) VALUES ('u1', 'api', 'nope', 1)`)
}

// A person has one row of preferences, with the system theme until they change it, and a progress
// row keeps skipping and finishing apart.
func TestMigration0010UserRows(t *testing.T) {
	s := openTemp(t)
	execAll(t, s, insertUser, `INSERT INTO user_preferences (user_id, updated_at) VALUES ('u1', 1)`,
		`INSERT INTO user_progress (user_id) VALUES ('u1')`)
	var theme, columns, sort string
	row := s.reader.QueryRowContext(testContext(t), "SELECT theme, list_columns_json, sort_json FROM user_preferences")
	if err := row.Scan(&theme, &columns, &sort); err != nil || theme != "system" || columns != "{}" || sort != "{}" {
		t.Errorf("defaults = %q, %q, %q, %v; want system, {}, {}", theme, columns, sort, err)
	}
	var skipped int
	if err := s.reader.QueryRowContext(testContext(t), "SELECT onboarding_skipped FROM user_progress").Scan(&skipped); err != nil || skipped != 0 {
		t.Errorf("onboarding_skipped = %d, %v; want 0", skipped, err)
	}
	wantRefused(t, s, "a second row of preferences for one person",
		`INSERT INTO user_preferences (user_id, updated_at) VALUES ('u1', 2)`)
	wantRefused(t, s, "preferences for a person who is not there",
		`INSERT INTO user_preferences (user_id, updated_at) VALUES ('nobody', 1)`)
	wantRefused(t, s, "a skipped flag that is not 0 or 1", `UPDATE user_progress SET onboarding_skipped = 2`)
}

// A database that was already in use keeps its people and its progress when it upgrades to 0010: the
// new columns are empty for them, which reads as no avatar and an onboarding that was not skipped.
func TestMigration0010KeepsWhatWasAlreadyThere(t *testing.T) {
	ctx := testContext(t)
	path := filepath.Join(t.TempDir(), "marshal.db")
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	writer, err := openPool(ctx, dsn(path, false), writers)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()

	if err := migrate(ctx, writer, migrationsThrough(t, 9), log); err != nil {
		t.Fatalf("apply the migrations through 0009: %v", err)
	}
	for _, statement := range []string{
		`INSERT INTO users (id, name, email, avatar_path, created_at, updated_at) VALUES ('u1', 'Ada', 'ada@example.com', 'old.png', 5, 6)`,
		`INSERT INTO user_progress (user_id, onboarding_step, onboarding_done_at, tutorial_skipped) VALUES ('u1', 3, 7, 1)`,
	} {
		if _, err := writer.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed %q: %v", statement, err)
		}
	}
	if err := migrate(ctx, writer, embeddedFiles(t), log); err != nil {
		t.Fatalf("apply the rest: %v", err)
	}

	var name, email, avatar string
	var avatarUpdated sql.NullInt64
	err = writer.QueryRowContext(ctx, "SELECT name, email, avatar_path, avatar_updated_at FROM users WHERE id = 'u1'").
		Scan(&name, &email, &avatar, &avatarUpdated)
	if err != nil || name != "Ada" || email != "ada@example.com" || avatar != "old.png" || avatarUpdated.Valid {
		t.Errorf("the person after the upgrade = %q, %q, %q, %+v, %v", name, email, avatar, avatarUpdated, err)
	}
	var step, onboardingSkipped, tutorialSkipped int
	var done sql.NullInt64
	err = writer.QueryRowContext(ctx,
		"SELECT onboarding_step, onboarding_done_at, onboarding_skipped, tutorial_skipped FROM user_progress WHERE user_id = 'u1'").
		Scan(&step, &done, &onboardingSkipped, &tutorialSkipped)
	if err != nil || step != 3 || !done.Valid || done.Int64 != 7 || onboardingSkipped != 0 || tutorialSkipped != 1 {
		t.Errorf("the progress after the upgrade = %d, %+v, %d, %d, %v", step, done, onboardingSkipped, tutorialSkipped, err)
	}
}
