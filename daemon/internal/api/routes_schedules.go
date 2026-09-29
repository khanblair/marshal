package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/schedules"
)

// The Schedules screen's own list over the API (docs/architecture.md sections 10 and 11.1; B8.1,
// build-plan 8.1). A schedule is one row: a brief or a job, when it runs, and what it does. The
// engine that runs them and the missed-run policy live in internal/schedules; these four routes are
// the list-get-save-delete shape N-lists elsewhere in this package use.
//
// Nothing here starts a run: saving a schedule only changes the row, and the cron reloads from the
// rows. A run that does not exist yet is not an error, and a schedule whose stored `when` cannot be
// turned into a cron spec still saves - ParseCron answers a best-effort spec rather than refusing -
// so a person never loses what they typed.

// scheduleKinds is what a schedule's kind may be, matching the screens' own two
// (apps/web/src/mock/settings-types.ts). A kind outside it is refused rather than stored, so a
// mistyped body cannot make a row no screen can draw.
func scheduleKinds() []string { return []string{"brief", "job"} }

// scheduleTriggers is what a schedule's trigger may be, matching the screens' own four.
func scheduleTriggers() []string { return []string{"Cron", "Interval", "One-time", "Event"} }

// listSchedules is GET /v1/schedules: every scheduled item, or one project's when ?project= is given,
// oldest first, stamped with the daemon's time so a screen can tell a stale answer from a fresh one.
func (s *Server) listSchedules(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	list, err := s.schedules.List(r.Context(), project)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, protocol.NewScheduleList(list, s.now()))
}

// scheduleRuns is GET /v1/schedules/{id}/runs: a schedule's own run history, newest first. A
// brief's own composed text is a run's details, and this is how a person reads one back until a
// real delivery channel exists (B8.5).
func (s *Server) scheduleRuns(w http.ResponseWriter, r *http.Request) {
	id, err := scheduleIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	runs, err := s.schedules.Runs(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, protocol.NewScheduleRunList(runs, s.now()))
}

// createSchedule is POST /v1/schedules: make a new schedule. The id is the daemon's to make, so an
// id in the body is ignored rather than trusted - a screen that wants to replace one uses the PUT.
func (s *Server) createSchedule(w http.ResponseWriter, r *http.Request) {
	var req protocol.SaveScheduleRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	req.ID = ""
	if err := validateSchedule(req); err != nil {
		s.writeError(w, err)
		return
	}
	saved, err := s.schedules.Save(r.Context(), req)
	if err != nil {
		s.writeError(w, scheduleError(err, req.ID))
		return
	}
	s.writeJSON(w, http.StatusOK, saved)
}

// saveSchedule is PUT /v1/schedules/{id}: edit one schedule. The id in the address is the one that
// is written, whatever the body says, so an edit cannot move a schedule to a second id by accident.
// An id Marshal has no row for is not found - the same answer an unknown card gets - rather than a
// half-written row.
func (s *Server) saveSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := scheduleIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.SaveScheduleRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	req.ID = id
	if err := validateSchedule(req); err != nil {
		s.writeError(w, err)
		return
	}
	saved, err := s.schedules.Save(r.Context(), req)
	if err != nil {
		s.writeError(w, scheduleError(err, id))
		return
	}
	s.writeJSON(w, http.StatusOK, saved)
}

// deleteSchedule is DELETE /v1/schedules/{id}: remove a schedule outright. Its run history goes with
// it (migration 0022's foreign key), because a run of a schedule that is gone belongs to nothing.
func (s *Server) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := scheduleIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.schedules.Delete(r.Context(), id); err != nil {
		s.writeError(w, translate(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateSchedule checks the parts of a schedule a screen cannot draw or the engine cannot run
// without. It is deliberately shallow: `when` is free text a person typed and stays exactly as it
// was typed, and every other field has a meaning the engine tolerates being empty.
func validateSchedule(req protocol.SaveScheduleRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return protocol.InvalidArgument("A schedule needs a name.")
	}
	if !contains(scheduleKinds(), req.Kind) {
		return protocol.InvalidArgument("A schedule's kind is either brief or job.").With("kind", req.Kind)
	}
	if !contains(scheduleTriggers(), req.Trigger) {
		return protocol.InvalidArgument("A schedule's trigger is Cron, Interval, One-time, or Event.").
			With("trigger", req.Trigger)
	}
	return nil
}

// contains reports whether list holds want. It is the small membership check several validators in
// this package need, kept local so importing slices is not needed for one call.
func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// scheduleIDOf reads a schedule's id from the address. A schedule id is opaque, like a card's, so an
// id that is not the right shape is not found rather than passed on.
func scheduleIDOf(r *http.Request) (string, error) {
	id := r.PathValue("id")
	if !protocol.ValidID(id) {
		return "", notFoundID("schedule", id)
	}
	return id, nil
}

// scheduleError turns what the schedules service returned into what the wire carries. The service's
// own ErrNotFound has no wire shape of its own, so it becomes the same not-found answer an unknown
// card gets, naming the id that was asked for; everything else is translated as it always is.
func scheduleError(err error, id string) error {
	if errors.Is(err, schedules.ErrNotFound) {
		return notFoundID("schedule", id)
	}
	return translate(err)
}
