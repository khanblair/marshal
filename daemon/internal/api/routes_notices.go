package api

import (
	"fmt"
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The notice routes and the sleep-settings routes (docs/architecture.md sections 5.2 and 10,
// checklist item B5.6, inventory N5).
//
// The notices themselves are the session manager's: a sleep notice names live sessions and the
// moment they sleep, so the service that owns those sessions is the one that owns the notice. That
// is why GET /v1/notices and its calls ask s.sessions and need no service of their own, and why the
// actions answer with a count: the app's toasts say "Kept 2 cards awake" and "3 cards asleep".
//
// The settings are the settings service's: they are values a person edits on the sleep screen and
// that outlive every session, so they are stored under their own key (internal/settings) rather than
// in the manager.
//
// The action body is one call rather than four addresses because the app has two buttons per card
// row and two for the group, and three of the four do the same thing to a different set of cards.

// listNotices is GET /v1/notices: every notice that is standing, for the bell and the notices panel.
// The list is always an array, so a client never has to handle both an empty list and a missing one.
func (s *Server) listNotices(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, protocol.NewNoticeList(s.sessions.Notices()))
}

// noticeAction is POST /v1/notices/{id}/actions: keep one card awake, sleep one card now, keep every
// card the notice names awake, or sleep them all now. It answers how many cards it changed.
//
// An id that names no notice answers zero rather than not found: a notice is derived state, and one
// that went away between the app drawing it and the person pressing a button on it (because its
// cards slept, or someone else kept them awake) is not a mistake worth an error. A card that cannot
// be slept - it started working, or it is waiting on the person - is refused with the sentence of
// section 5.1, because that one is about the card the person is looking at.
func (s *Server) noticeAction(w http.ResponseWriter, r *http.Request) {
	id, err := noticeIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.NoticeActionRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	count, err := s.runNoticeAction(r, id, req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, protocol.NoticeActionResult{Cards: count})
}

// runNoticeAction carries out one notice action. Keeping the switch in its own function keeps the
// handler to its own job (reading the request and writing the answer), and makes the four cases
// read as the four buttons they are.
func (s *Server) runNoticeAction(r *http.Request, id string, req protocol.NoticeActionRequest) (int, error) {
	switch req.Action {
	case protocol.NoticeActionKeepAwake:
		if _, err := s.sessions.KeepAwake(r.Context(), req.CardID); err != nil {
			return 0, err
		}
		return 1, nil
	case protocol.NoticeActionSleepNow:
		if err := s.sessions.Sleep(r.Context(), req.CardID); err != nil {
			return 0, err
		}
		return 1, nil
	case protocol.NoticeActionKeepAll:
		return s.sessions.KeepAllAwake(r.Context(), id)
	case protocol.NoticeActionSleepAll:
		return s.sessions.SleepAll(r.Context(), id)
	}
	return 0, noSuchNoticeAction(req.Action)
}

// dismissNotice is DELETE /v1/notices/{id}: clear a notice without changing any card. The cards
// stay awake, held off the idle timer for the keep-awake setting's length, so the notice does not
// come straight back. Dismissing a notice that is already gone is not an error.
func (s *Server) dismissNotice(w http.ResponseWriter, r *http.Request) {
	id, err := noticeIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.sessions.DismissNotice(r.Context(), id); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeNoContent(w)
}

// getSleepSettings is GET /v1/settings/sleep: the idle time, the warning time, the keep-awake time,
// what happens to awake cards at restart, and where warnings go. A fresh install answers with the
// shipped defaults rather than a not-found, which is what its own screen shows.
func (s *Server) getSleepSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.sleepSettings.Sleep(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, cfg)
}

// setSleepSettings is PUT /v1/settings/sleep: save the whole record, so the form is one write. The
// values are checked first, and a value the screen would not offer is refused with the sentence the
// form shows rather than stored and read back on the next open.
func (s *Server) setSleepSettings(w http.ResponseWriter, r *http.Request) {
	var req protocol.SleepSettings
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	saved, err := s.sleepSettings.SetSleep(r.Context(), req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, saved)
}

// noSuchNoticeAction refuses an action name this daemon has no meaning for. It is an invalid
// argument and not a refusal, because nothing about the notice or the cards is wrong: the request
// asked for something Marshal does not do.
func noSuchNoticeAction(action string) error {
	return protocol.InvalidArgument(fmt.Sprintf("There is no %q action on a notice, so nothing was"+
		" changed. A notice can be kept awake, slept now, kept all awake, or slept all now.", action))
}

// noticeIDOf reads the notice id of the address. A notice id is "<kind>:<projectId>" and is checked
// only for being present: the id names a group the manager holds, and an id it does not hold is an
// empty group, not a malformed address. Cutting a very long id keeps a bad address out of the error.
func noticeIDOf(r *http.Request) (string, error) {
	id := r.PathValue("id")
	if id == "" || len(id) > maxEchoedIDBytes {
		return "", protocol.InvalidArgument("A notice id is needed in the address.")
	}
	return id, nil
}
