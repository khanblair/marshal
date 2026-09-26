package api

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"

	"github.com/khanblair/marshal/daemon/internal/accounts"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The person using Marshal (docs/backend-checklist.md B2.2, B2.5, and B2.13): their profile and
// avatar, the users list, their onboarding and tour progress, and the preferences that follow them
// between devices. The rules are in internal/accounts. These handlers find the user the token
// belongs to, read the body, and write the answer. A token belongs to one user, so a request never
// names whose profile it means: there is no /v1/users/{id} that changes another person.

// caller is the user the request's token belongs to. It answers for the handler and returns false
// when there is no caller. A protected route always has one, so that cannot happen; if it did the
// answer must not be a guess.
func (s *Server) caller(w http.ResponseWriter, r *http.Request) (string, bool) {
	who, ok := Principal(r.Context())
	if !ok {
		s.writeError(w, errUnauthorized())
		return "", false
	}
	return who.UserID, true
}

// serveFor answers a route that reads or does something for the caller: it finds the user, calls
// the service with them, and sends what the service made with a 200. It is the whole body of the
// routes that take no body.
func serveFor[T any](s *Server, w http.ResponseWriter, r *http.Request, call func(context.Context, string) (T, error)) {
	user, ok := s.caller(w, r)
	if !ok {
		return
	}
	value, err := call(r.Context(), user)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, value)
}

// changeFor answers a route that changes something for the caller: it finds the user, reads the
// JSON body into a request, calls the service with both, and sends what the service made with a
// 200. It is the whole body of the PATCH routes.
func changeFor[R, T any](s *Server, w http.ResponseWriter, r *http.Request, call func(context.Context, string, R) (T, error)) {
	user, ok := s.caller(w, r)
	if !ok {
		return
	}
	var req R
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	value, err := call(r.Context(), user, req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, value)
}

// getMe is GET /v1/me: the profile of the person the token belongs to.
func (s *Server) getMe(w http.ResponseWriter, r *http.Request) {
	serveFor(s, w, r, s.accounts.Profile)
}

// updateMe is PATCH /v1/me: change the name, the email, or the time zone.
func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	changeFor(s, w, r, s.accounts.UpdateProfile)
}

// setAvatar is POST /v1/me/avatar. The body is the image itself, and its Content-Type says which
// kind: PNG, JPEG, or WebP, of at most accounts.MaxAvatarBytes. It is not JSON, so the route is
// registered without the JSON body rules and applies its own limit here. The answer is the new
// profile, whose avatarUrl is the address of the image.
func (s *Server) setAvatar(w http.ResponseWriter, r *http.Request) {
	user, ok := s.caller(w, r)
	if !ok {
		return
	}
	if r.ContentLength > accounts.MaxAvatarBytes {
		discardUpload(r)
		s.writeError(w, accounts.AvatarTooLarge())
		return
	}
	// A header that cannot be read is an empty kind, which the service refuses as not an image.
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	body := http.MaxBytesReader(w, r.Body, accounts.MaxAvatarBytes)
	profile, err := s.accounts.SetAvatar(r.Context(), user, mediaType, body)
	if err != nil {
		discardUpload(r)
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, profile)
}

// discardUpload reads what is left of an upload that is being refused, up to the ceiling the router
// puts on every upload. A client that is still sending when the answer arrives may be cut off
// before it reads the answer, and then it shows a network error and not the reason.
func discardUpload(r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
}

// removeAvatar is DELETE /v1/me/avatar. A person with no avatar is not an error: the answer is
// their profile either way.
func (s *Server) removeAvatar(w http.ResponseWriter, r *http.Request) {
	serveFor(s, w, r, s.accounts.RemoveAvatar)
}

// The two cache rules of an avatar. The address carries the version of the image, so a request that
// names the current version can be kept for a year, and any other is checked each time.
const (
	avatarCacheForever = "private, max-age=31536000, immutable"
	avatarCacheCheck   = "private, no-cache"
)

// getAvatar is GET /v1/users/{id}/avatar: the image itself, with its own kind. It needs the token
// like every other route, so a client fetches it with the Authorization header and shows the
// bytes; a bare <img> tag cannot. A user with no avatar is not found.
func (s *Server) getAvatar(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !protocol.ValidID(id) {
		s.writeError(w, notFoundID("avatar", id))
		return
	}
	image, err := s.accounts.Avatar(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	file, err := os.Open(image.Path)
	if errors.Is(err, os.ErrNotExist) {
		// The database says there is an image and the file is gone. Serving nothing is the honest
		// answer, and the person can choose the image again.
		s.log.Warn("an avatar file is missing", "user_id", id)
		s.writeError(w, notFoundID("avatar", id))
		return
	}
	if err != nil {
		s.writeError(w, err)
		return
	}
	defer func() { _ = file.Close() }()
	cache := avatarCacheCheck
	if r.URL.Query().Get("v") == strconv.FormatInt(image.Version, 10) {
		cache = avatarCacheForever
	}
	w.Header().Set("Content-Type", image.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", cache)
	http.ServeContent(w, r, "", image.UpdatedAt, file)
}

// listUsers is GET /v1/users: everyone who can be put on a card, for the member pickers. Solo use
// lists the owner only.
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.accounts.Users(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, snapshot)
}

// getProgress is GET /v1/me/progress: how far the person is with onboarding and with the tour.
func (s *Server) getProgress(w http.ResponseWriter, r *http.Request) {
	serveFor(s, w, r, s.accounts.Progress)
}

// updateProgress is PATCH /v1/me/progress: save the screen onboarding is on, or finish, skip, or
// replay onboarding or the tour.
func (s *Server) updateProgress(w http.ResponseWriter, r *http.Request) {
	changeFor(s, w, r, s.accounts.UpdateProgress)
}

// getPreferences is GET /v1/me/preferences: the theme, the List columns, the sort, and each
// project's last view, filters, and swimlane.
func (s *Server) getPreferences(w http.ResponseWriter, r *http.Request) {
	serveFor(s, w, r, s.accounts.Preferences)
}

// updatePreferences is PATCH /v1/me/preferences: save what the body sets and leave the rest.
func (s *Server) updatePreferences(w http.ResponseWriter, r *http.Request) {
	changeFor(s, w, r, s.accounts.UpdatePreferences)
}

// resetFirstLaunch is POST /v1/dev/reset-first-launch: onboarding goes back to its first screen and
// the tour to pending, so a developer can walk through first launch again. The route is registered
// only on a dev daemon (needsDevMode), so on a normal one the address does not exist.
func (s *Server) resetFirstLaunch(w http.ResponseWriter, r *http.Request) {
	serveFor(s, w, r, s.accounts.ResetFirstLaunch)
}
