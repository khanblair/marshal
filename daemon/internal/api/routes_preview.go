package api

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A card's live preview (docs/architecture.md section 11.2 and N10, docs/backend-checklist.md B6.6
// and B6.7, build-plan 6.6 and 6.7, docs/marshal-product-scope.md 15.1 and 15.2): one dev server
// per card, on its own port, with the before and after screenshots taken of it.
//
// The routes are the daemon's side of the Preview tab. Reading the tab starts nothing, so a person
// looking at the preview does not start a dev server on their machine by looking: only the start
// route runs anything, and only the stop route stops it.

// getPreview is GET /v1/cards/{id}/preview: the card's preview as it is now. It starts nothing.
func (s *Server) getPreview(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	answer, err := s.preview.Snapshot(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}

// startPreview is POST /v1/cards/{id}/preview/start: run the project's dev command for the card in
// the card's own worktree, on a port picked for it. The answer is the preview as it is now, which is
// `starting` until the dev server answers - so a card with no worktree or a project with no dev
// command is refused in a plain sentence rather than shown a spinner that never ends.
func (s *Server) startPreview(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	answer, err := s.preview.Start(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}

// stopPreview is POST /v1/cards/{id}/preview/stop: stop the card's dev server. Stopping a preview
// that is not running is not an error: the person asked for the state they wanted and have it.
func (s *Server) stopPreview(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	answer, err := s.preview.Stop(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}

// takePreviewShot is POST /v1/cards/{id}/preview/shots: take one half of the before and after pair of
// a running preview. A browser Marshal cannot find is not an error: the answer says the check was
// skipped and why, in a sentence, because a check that never ran must never read as passed
// (docs/library-docs.md).
func (s *Server) takePreviewShot(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.PreviewShotRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	// Driving a browser is slower than any other route here, so this one gets the longer window to
	// write its answer in and is not cut off while a cold browser is still starting.
	s.allowSlowAnswer(w)
	answer, err := s.preview.Screenshot(r.Context(), id, req.Kind)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}

// The two cache rules of a screenshot, the same as an avatar's. The address carries the version of
// the image, so a request that names the current version can be kept for a year, and any other is
// checked each time.
const (
	previewShotCacheForever = "private, max-age=31536000, immutable"
	previewShotCacheCheck   = "private, no-cache"
	// previewShotSuffix is the image kind a screenshot is served as. The address names the file, so
	// the kind word is followed by this.
	previewShotSuffix = ".png"
)

// getPreviewShot is GET /v1/cards/{id}/preview/shots/{file}: the image itself, with its own kind.
// Like an avatar it needs the token, so a client fetches it with the Authorization header and shows
// the bytes; a bare <img> tag cannot. A card with no such shot is not found.
func (s *Server) getPreviewShot(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	kind, err := previewShotKindOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	path, modified, err := s.preview.ShotFile(id, kind)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	file, err := os.Open(path)
	if err != nil {
		// The state says there is a shot and the file is gone. Serving nothing is the honest
		// answer, and the person can take the screenshot again.
		s.log.Warn("a preview screenshot file is missing", "card_id", id, "kind", string(kind))
		s.writeError(w, notFoundID("screenshot", id))
		return
	}
	defer func() { _ = file.Close() }()
	cache := previewShotCacheCheck
	if r.URL.Query().Get("v") == strconv.FormatInt(modified.UnixMilli(), 10) {
		cache = previewShotCacheForever
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", cache)
	http.ServeContent(w, r, "", modified, file)
}

// previewShotKindOf reads a screenshot kind out of the last part of the address. The address names
// a file, so anything that is not one of the two kinds, or does not name a PNG, is not found rather
// than passed on.
func previewShotKindOf(r *http.Request) (protocol.PreviewShotKind, error) {
	name := r.PathValue("file")
	word, isPNG := strings.CutSuffix(name, previewShotSuffix)
	kind := protocol.PreviewShotKind(word)
	if !isPNG || !kind.Valid() {
		return "", notFoundID("screenshot", name)
	}
	return kind, nil
}
