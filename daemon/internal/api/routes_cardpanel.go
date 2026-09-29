package api

import (
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/khanblair/marshal/daemon/internal/cardpanel"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// What a card's panel holds beyond the card (docs/backend-checklist.md B10.2, B10.5, B10.6):
// acceptance checks, named checklists, comments with attachments, and the people on the card. Every
// route reads the card's id and the caller, calls internal/cardpanel, and writes the whole list the
// call answers, so a screen redraws from one answer.

// panelIDOf reads one opaque id of the address, as not found when it is not the shape of one.
func panelIDOf(r *http.Request, name, what string) (string, error) {
	id := r.PathValue(name)
	if !protocol.ValidID(id) {
		return "", notFoundID(what, id)
	}
	return id, nil
}

// actorOf is the person behind the request.
func actorOf(r *http.Request) (cardpanel.Actor, error) {
	caller, ok := Principal(r.Context())
	if !ok {
		return cardpanel.Actor{}, errUnauthorized()
	}
	return cardpanel.Actor{UserID: caller.UserID}, nil
}

// panelCall runs one panel call and writes its answer. `do` gets the card's id.
func panelCall[T any](s *Server, w http.ResponseWriter, r *http.Request, do func(cardID string) (T, error)) {
	cardID, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	out, err := do(cardID)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, out)
}

func (s *Server) getCardChecks(w http.ResponseWriter, r *http.Request) {
	panelCall(s, w, r, func(id string) (protocol.CardCheckList, error) { return s.cardPanel.Checks(r.Context(), id) })
}

func (s *Server) addCardCheck(w http.ResponseWriter, r *http.Request) {
	var req protocol.AddCardCheckRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	panelCall(s, w, r, func(id string) (protocol.CardCheckList, error) { return s.cardPanel.AddCheck(r.Context(), id, req) })
}

func (s *Server) removeCardCheck(w http.ResponseWriter, r *http.Request) {
	check, err := panelIDOf(r, "check", "check")
	if err != nil {
		s.writeError(w, err)
		return
	}
	panelCall(s, w, r, func(id string) (protocol.CardCheckList, error) {
		return s.cardPanel.RemoveCheck(r.Context(), id, check)
	})
}

func (s *Server) runCardChecks(w http.ResponseWriter, r *http.Request) {
	panelCall(s, w, r, func(id string) (protocol.CardCheckList, error) { return s.cardPanel.RunChecks(r.Context(), id) })
}

func (s *Server) getChecklists(w http.ResponseWriter, r *http.Request) {
	panelCall(s, w, r, func(id string) (protocol.ChecklistList, error) { return s.cardPanel.Checklists(r.Context(), id) })
}

func (s *Server) createChecklist(w http.ResponseWriter, r *http.Request) {
	var req protocol.CreateChecklistRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	panelCall(s, w, r, func(id string) (protocol.ChecklistList, error) {
		return s.cardPanel.CreateChecklist(r.Context(), id, req)
	})
}

func (s *Server) updateChecklist(w http.ResponseWriter, r *http.Request) {
	list, err := panelIDOf(r, "list", "checklist")
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.UpdateChecklistRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	panelCall(s, w, r, func(id string) (protocol.ChecklistList, error) {
		return s.cardPanel.UpdateChecklist(r.Context(), id, list, req)
	})
}

func (s *Server) deleteChecklist(w http.ResponseWriter, r *http.Request) {
	list, err := panelIDOf(r, "list", "checklist")
	if err != nil {
		s.writeError(w, err)
		return
	}
	panelCall(s, w, r, func(id string) (protocol.ChecklistList, error) {
		return s.cardPanel.DeleteChecklist(r.Context(), id, list)
	})
}

func (s *Server) addChecklistItem(w http.ResponseWriter, r *http.Request) {
	list, err := panelIDOf(r, "list", "checklist")
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.AddChecklistItemRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	panelCall(s, w, r, func(id string) (protocol.ChecklistList, error) {
		return s.cardPanel.AddItem(r.Context(), id, list, req)
	})
}

func (s *Server) tickChecklistItem(w http.ResponseWriter, r *http.Request) {
	list, err := panelIDOf(r, "list", "checklist")
	if err != nil {
		s.writeError(w, err)
		return
	}
	item, err := panelIDOf(r, "item", "checklist item")
	if err != nil {
		s.writeError(w, err)
		return
	}
	by, err := actorOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.TickChecklistItemRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	panelCall(s, w, r, func(id string) (protocol.ChecklistList, error) {
		return s.cardPanel.Tick(r.Context(), id, list, item, req.Done, by)
	})
}

func (s *Server) removeChecklistItem(w http.ResponseWriter, r *http.Request) {
	list, err := panelIDOf(r, "list", "checklist")
	if err != nil {
		s.writeError(w, err)
		return
	}
	item, err := panelIDOf(r, "item", "checklist item")
	if err != nil {
		s.writeError(w, err)
		return
	}
	panelCall(s, w, r, func(id string) (protocol.ChecklistList, error) {
		return s.cardPanel.RemoveItem(r.Context(), id, list, item)
	})
}

func (s *Server) getComments(w http.ResponseWriter, r *http.Request) {
	panelCall(s, w, r, func(id string) (protocol.CommentList, error) { return s.cardPanel.Comments(r.Context(), id) })
}

func (s *Server) postComment(w http.ResponseWriter, r *http.Request) {
	by, err := actorOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.PostCommentRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	panelCall(s, w, r, func(id string) (protocol.CommentList, error) {
		return s.cardPanel.Post(r.Context(), id, req, by)
	})
}

func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	comment, err := panelIDOf(r, "comment", "comment")
	if err != nil {
		s.writeError(w, err)
		return
	}
	by, err := actorOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	panelCall(s, w, r, func(id string) (protocol.CommentList, error) {
		return s.cardPanel.DeleteComment(r.Context(), id, comment, by)
	})
}

// getAttachment serves a kept file. It is always a download, never something the browser runs, and
// only pictures of a few known types are shown in place.
func (s *Server) getAttachment(w http.ResponseWriter, r *http.Request) {
	cardID, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	attachment, err := panelIDOf(r, "attachment", "attachment")
	if err != nil {
		s.writeError(w, err)
		return
	}
	file, err := s.cardPanel.Attachment(r.Context(), cardID, attachment)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	f, err := os.Open(file.Path)
	if err != nil {
		s.writeError(w, notFoundID("attachment", attachment))
		return
	}
	defer func() { _ = f.Close() }()
	kind, disposition := "application/octet-stream", "attachment"
	switch file.MimeType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		kind, disposition = file.MimeType, "inline"
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.Name}))
	_, _ = io.Copy(w, f)
}

func (s *Server) getCardMembers(w http.ResponseWriter, r *http.Request) {
	panelCall(s, w, r, func(id string) (protocol.CardMembers, error) { return s.cardPanel.Members(r.Context(), id) })
}

func (s *Server) addCardMember(w http.ResponseWriter, r *http.Request) {
	user, err := panelIDOf(r, "user", "person")
	if err != nil {
		s.writeError(w, err)
		return
	}
	panelCall(s, w, r, func(id string) (protocol.CardMembers, error) { return s.cardPanel.AddMember(r.Context(), id, user) })
}

func (s *Server) removeCardMember(w http.ResponseWriter, r *http.Request) {
	user, err := panelIDOf(r, "user", "person")
	if err != nil {
		s.writeError(w, err)
		return
	}
	panelCall(s, w, r, func(id string) (protocol.CardMembers, error) { return s.cardPanel.RemoveMember(r.Context(), id, user) })
}
