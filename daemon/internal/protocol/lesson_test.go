package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The wire shape of a project's lesson (task 7.6, 7.13): the markdown the lessons screen shows,
// where its file lives in the vault, its title and slug, and who wrote it.

// sampleLesson is a lesson an agent left after tracking down a flaky CI run.
func sampleLesson() protocol.Lesson {
	return protocol.NewLesson(
		"small-repo", "ci-is-flaky-on-windows", "CI is flaky on Windows",
		"small-repo/lessons/ci-is-flaky-on-windows.md",
		"# CI is flaky on Windows\n\nRetry the flaky step once before failing the run.\n",
		protocol.NoteAuthorAgent, providersNow,
	)
}

func TestLessonGolden(t *testing.T) {
	testutil.Golden(t, "lesson", sampleLesson())
}

// A lesson always carries a save time: there is no "nothing saved yet" state the way a card's note
// has, because a lesson that was never saved has no slug to be read back by.
func TestALessonAlwaysCarriesItsTime(t *testing.T) {
	encoded, err := json.Marshal(sampleLesson())
	if err != nil {
		t.Fatal(err)
	}
	got := string(encoded)
	if !strings.Contains(got, `"updatedAt":"2026-09-27T09:30:00.000Z"`) {
		t.Errorf("a lesson encoded as %s, want the daemon's time", got)
	}
	if strings.Contains(got, `"updatedAt":null`) {
		t.Error("a lesson's time must never be null")
	}
}

// A lesson's path is relative to the vault root, the same rule a card note's path follows and for
// the same reason: the vault lives wherever the person put it.
func TestALessonsPathIsRelative(t *testing.T) {
	if strings.HasPrefix(sampleLesson().Path, "/") {
		t.Error("the lesson's path is absolute, want it relative to the vault root")
	}
}

// A lesson's body round-trips through SaveLessonRequest unchanged, the same guarantee
// SaveNoteRequest gives a card's note: the file is what a person edits in Obsidian, so its
// punctuation and newlines must survive.
func TestASaveLessonRequestRoundTrips(t *testing.T) {
	req := protocol.SaveLessonRequest{Title: sampleLesson().Title, Body: sampleLesson().Body}
	encoded, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var back protocol.SaveLessonRequest
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatal(err)
	}
	if back != req {
		t.Errorf("the lesson request came back as %+v, want %+v", back, req)
	}
}
