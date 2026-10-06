package api

import (
	"context"
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The routes of the four Google file connections, which are set up through the integration routes.
// One that is not connected, or whose access Google no longer accepts, answers a refusal that
// names it (googleError).

// listGoogleFiles is GET /v1/google/files?kind=doc|sheet|slides|file: what Marshal made, newest first.
// With no kind it is every kind but folders.
func (s *Server) listGoogleFiles(w http.ResponseWriter, r *http.Request) {
	kind := protocol.GoogleFileKind(r.URL.Query().Get("kind"))
	files, err := s.integrations.GoogleFiles(r.Context(), kind)
	if err != nil {
		s.writeError(w, s.googleError(err))
		return
	}
	s.writeJSON(w, http.StatusOK, files)
}

// createGoogleDoc is POST /v1/google/docs: a Google Doc in Marshal's folder.
func (s *Server) createGoogleDoc(w http.ResponseWriter, r *http.Request) {
	createGoogleFile(s, w, r, s.integrations.CreateGoogleDoc)
}

// createGoogleSheet is POST /v1/google/sheets: a Google Sheet in Marshal's folder.
func (s *Server) createGoogleSheet(w http.ResponseWriter, r *http.Request) {
	createGoogleFile(s, w, r, s.integrations.CreateGoogleSheet)
}

// createGoogleSlides is POST /v1/google/slides: a Google Slides presentation in Marshal's folder.
func (s *Server) createGoogleSlides(w http.ResponseWriter, r *http.Request) {
	createGoogleFile(s, w, r, s.integrations.CreateGoogleSlides)
}

// uploadGoogleFile is POST /v1/google/drive/files: a plain file in Marshal's folder, as it is.
func (s *Server) uploadGoogleFile(w http.ResponseWriter, r *http.Request) {
	createGoogleFile(s, w, r, s.integrations.UploadGoogleFile)
}

// createGoogleFile reads the body, makes the file, and answers 201 with it and its link.
func createGoogleFile[T any](s *Server, w http.ResponseWriter, r *http.Request, create func(context.Context, T) (protocol.GoogleFile, error)) {
	var req T
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	file, err := create(r.Context(), req)
	if err != nil {
		s.writeError(w, s.googleError(err))
		return
	}
	s.writeJSON(w, http.StatusCreated, file)
}

// readGoogleLink is POST /v1/google/read: the Google Doc, Sheet or Slides presentation a pasted link
// points to, read into markdown. Only the file's id is taken from the link.
func (s *Server) readGoogleLink(w http.ResponseWriter, r *http.Request) {
	var req protocol.ReadGoogleLinkRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	content, err := s.integrations.ReadGoogleLink(r.Context(), req)
	if err != nil {
		s.writeError(w, s.googleError(err))
		return
	}
	s.writeJSON(w, http.StatusOK, content)
}
