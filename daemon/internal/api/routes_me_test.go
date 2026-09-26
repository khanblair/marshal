package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/coder/websocket"

	"github.com/khanblair/marshal/daemon/internal/accounts"
	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The person's routes (docs/backend-checklist.md B2.2, B2.5, and B2.13) over the real server, the
// real store, and the real event bus. The rules themselves are tested in internal/accounts; these
// tests are about what the routes read, what they answer, and what they announce.

// connect opens the event stream on the topics and waits for the daemon's first frame, the resync a
// new connection gets. It is sent after the subscription is made, so nothing published from here on
// can be missed by the test.
func (st *stack) connect(topics ...protocol.Topic) *wire {
	st.t.Helper()
	w := st.dial(topics...)
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	for len(w.resyncs) == 0 {
		if err := w.read(ctx); err != nil {
			st.t.Fatalf("waiting for the first frame: %v", err)
		}
	}
	return w
}

// meOf reads the me.updated event a wire hears next.
func meOf(t *testing.T, w *wire) protocol.MeUpdatedEventData {
	t.Helper()
	seen := w.until(ofType(protocol.EventTypeMeUpdated))
	last := seen[len(seen)-1]
	if last.Topic != protocol.MeTopic {
		t.Fatalf("me.updated came on %s, want the me topic", last.Topic)
	}
	return dataOf[protocol.MeUpdatedEventData](t, last)
}

// onePixelPNG is a real PNG image of one pixel.
func onePixelPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("encode a PNG: %v", err)
	}
	return buf.Bytes()
}

// upload sends an image to the avatar route, with the kind it says it is.
func (st *stack) upload(kind string, body []byte) reply {
	st.t.Helper()
	req := st.newRequest(http.MethodPost, "/v1/me/avatar", body)
	req.Header.Set("Content-Type", kind)
	req.Header.Set("Authorization", "Bearer "+st.token)
	return st.send(req)
}

func TestTheProfileRoutesReadAndChangeTheProfile(t *testing.T) {
	st := newStack(t)
	r := st.do(http.MethodGet, "/v1/me", nil).want(t, http.StatusOK)
	sameShape(t, "profile", r.Body)
	before := decode[protocol.Profile](t, r)
	if before.Name != "Owner" || before.Initials != "O" || before.AvatarURL != nil {
		t.Errorf("a new install's profile = %+v", before)
	}
	whoami := decode[protocol.WhoAmI](t, st.do(http.MethodGet, "/v1/auth/whoami", nil).want(t, http.StatusOK))
	if before.ID != whoami.UserID {
		t.Errorf("the profile is of %s, and the token belongs to %s", before.ID, whoami.UserID)
	}

	name, email, zone := "Ada Okafor", "ada@example.com", "Europe/London"
	r = st.do(http.MethodPatch, "/v1/me", protocol.UpdateProfileRequest{Name: &name, Email: &email, TimeZone: &zone}).want(t, http.StatusOK)
	sameShape(t, "profile", r.Body)
	saved := decode[protocol.Profile](t, r)
	if saved.Name != name || saved.Email != email || saved.TimeZone != zone || saved.Initials != "AO" {
		t.Errorf("the saved profile = %+v", saved)
	}
	if got := decode[protocol.Profile](t, st.do(http.MethodGet, "/v1/me", nil).want(t, http.StatusOK)); got.Name != name {
		t.Errorf("the profile read back = %+v", got)
	}

	// The daemon starts again on the same data, and the profile is there.
	st.restart()
	if got := decode[protocol.Profile](t, st.do(http.MethodGet, "/v1/me", nil).want(t, http.StatusOK)); got.Name != name || got.Email != email {
		t.Errorf("the profile after a restart = %+v", got)
	}
}

func TestTheProfileRoutesRefuseWhatIsNotAllowed(t *testing.T) {
	st := newStack(t)
	empty := ""
	got := st.do(http.MethodPatch, "/v1/me", protocol.UpdateProfileRequest{Name: &empty}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if got.Message != "Enter a name. It shows on cards you comment on." {
		t.Errorf("message = %q", got.Message)
	}
	st.do(http.MethodPatch, "/v1/me", `{"nickname":"Ada"}`).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPatch, "/v1/me", `not json`).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if profile := decode[protocol.Profile](t, st.do(http.MethodGet, "/v1/me", nil).want(t, http.StatusOK)); profile.Name != "Owner" {
		t.Errorf("a refused change was kept: %+v", profile)
	}
	st.do(http.MethodPut, "/v1/me", nil).apiError(t, http.StatusMethodNotAllowed, protocol.ErrorCodeMethodNotAllowed)
}

// Two devices of one person: a change made on one is heard by the other, whatever it changes, and a
// connection that does not follow the `me` topic hears none of it.
func TestAChangeOnOneDeviceIsHeardOnAnother(t *testing.T) {
	st := newStack(t)
	first, second := st.connect(protocol.MeTopic), st.connect(protocol.MeTopic)
	elsewhere := st.connect(protocol.HomeTopic)

	name := "Ada Okafor"
	st.do(http.MethodPatch, "/v1/me", protocol.UpdateProfileRequest{Name: &name}).want(t, http.StatusOK)
	for i, w := range []*wire{first, second} {
		if me := meOf(t, w); me.Profile.Name != name {
			t.Errorf("device %d heard the name %q, want %q", i+1, me.Profile.Name, name)
		}
	}

	st.do(http.MethodPatch, "/v1/me/preferences", protocol.UpdatePreferencesRequest{Theme: ptr(protocol.ThemeDark)}).want(t, http.StatusOK)
	for i, w := range []*wire{first, second} {
		me := meOf(t, w)
		if me.Preferences.Theme != protocol.ThemeDark || me.Profile.Name != name {
			t.Errorf("device %d heard %+v, want the dark theme and the new name together", i+1, me)
		}
	}

	st.do(http.MethodPatch, "/v1/me/progress", protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Step: ptr(2)},
	}).want(t, http.StatusOK)
	for i, w := range []*wire{first, second} {
		if me := meOf(t, w); me.Progress.Onboarding.Step != 2 {
			t.Errorf("device %d heard step %d, want 2", i+1, me.Progress.Onboarding.Step)
		}
	}

	// A change that changes nothing is not announced: the next event a device hears is the next
	// real change.
	st.do(http.MethodPatch, "/v1/me/preferences", protocol.UpdatePreferencesRequest{Theme: ptr(protocol.ThemeDark)}).want(t, http.StatusOK)
	st.do(http.MethodPatch, "/v1/me/preferences", protocol.UpdatePreferencesRequest{Theme: ptr(protocol.ThemeLight)}).want(t, http.StatusOK)
	if me := meOf(t, second); me.Preferences.Theme != protocol.ThemeLight {
		t.Errorf("the device heard %+v, want the change to light and nothing before it", me.Preferences)
	}

	// Something happens on the home topic, so the connection that follows only that has events to
	// read, and none of them is a change of the person.
	st.addProject("small-repo")
	for _, event := range elsewhere.until(ofType(protocol.EventTypeProjectCreated)) {
		if event.Type == protocol.EventTypeMeUpdated {
			t.Errorf("a connection that follows only home heard %s", event.Type)
		}
	}
}

func TestThePreferencesRoutes(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")

	r := st.do(http.MethodGet, "/v1/me/preferences", nil).want(t, http.StatusOK)
	wantPreferenceKeys(t, r.Body)
	if got := decode[protocol.Preferences](t, r); got.Theme != protocol.ThemeSystem || len(got.Projects) != 0 || got.ListColumns == nil {
		t.Errorf("a new install's preferences = %+v", got)
	}

	view := decode[protocol.SavedView](t, st.do(http.MethodPost, "/v1/projects/"+project.ID+"/saved-views",
		protocol.CreateSavedViewRequest{Name: "Needs me"}).want(t, http.StatusCreated))
	lanes := []string{"role:Tester"}
	in := protocol.UpdatePreferencesRequest{
		Theme:       ptr(protocol.ThemeDark),
		ListColumns: map[string]bool{"pkg": true},
		Sort:        &protocol.UpdateSortPreferences{List: &protocol.SortOrder{Key: "id", Direction: protocol.SortDirectionDesc}},
		Projects: map[string]protocol.UpdateProjectPreferences{project.ID: {
			LastView: ptr(protocol.ProjectViewList), Filters: &[]protocol.Filter{{Key: protocol.FilterKeyStatus, Value: "needs"}},
			Query: ptr("cache"), Swimlane: ptr(protocol.SwimlaneRole), CollapsedLanes: &lanes, ShowAllDone: ptr(true), SavedViewID: &view.ID,
		}},
	}
	r = st.do(http.MethodPatch, "/v1/me/preferences", in).want(t, http.StatusOK)
	wantPreferenceKeys(t, r.Body)
	saved := decode[protocol.Preferences](t, r)
	got := saved.Projects[project.ID]
	if saved.Theme != protocol.ThemeDark || !saved.ListColumns["pkg"] || got.LastView != protocol.ProjectViewList ||
		got.Query != "cache" || got.SavedViewID == nil || *got.SavedViewID != view.ID || !got.ShowAllDone {
		t.Errorf("the saved preferences = %+v", saved)
	}

	// The daemon starts again on the same data, and every preference is there.
	st.restart()
	if again := decode[protocol.Preferences](t, st.do(http.MethodGet, "/v1/me/preferences", nil).want(t, http.StatusOK)); again.Projects[project.ID].Query != "cache" ||
		again.Theme != protocol.ThemeDark {
		t.Errorf("the preferences after a restart = %+v", again)
	}

	// A saved view that is deleted leaves no saved view in use, and the project's other preferences.
	st.do(http.MethodDelete, "/v1/saved-views/"+view.ID, nil).want(t, http.StatusNoContent)
	after := decode[protocol.Preferences](t, st.do(http.MethodGet, "/v1/me/preferences", nil).want(t, http.StatusOK))
	if after.Projects[project.ID].SavedViewID != nil || after.Projects[project.ID].Swimlane != protocol.SwimlaneRole {
		t.Errorf("after the view was deleted the project's preferences = %+v", after.Projects[project.ID])
	}
}

func ptr[T any](value T) *T { return &value }

// wantPreferenceKeys checks the shape of a preferences answer. The golden file cannot be compared
// by shape, because two of its objects are keyed by things that differ from one daemon to the next,
// the List columns and the projects, so the fixed keys are checked here and the types are checked by
// decoding into the wire type, which refuses a key it does not have.
func wantPreferenceKeys(t *testing.T, body []byte) {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		t.Fatalf("decode the preferences: %v", err)
	}
	for _, key := range []string{"theme", "listColumns", "sort", "projects"} {
		if _, ok := top[key]; !ok || len(top) != 4 {
			t.Fatalf("the preferences have the keys %v, want theme, listColumns, sort, and projects: %s", keysOf(top), body)
		}
	}
	var sort map[string]json.RawMessage
	if err := json.Unmarshal(top["sort"], &sort); err != nil || len(sort) != 2 || sort["agents"] == nil || sort["list"] == nil {
		t.Errorf("the sort is %s, want both tables, each null or an order", top["sort"])
	}
	if strings.HasPrefix(string(top["listColumns"]), "null") || strings.HasPrefix(string(top["projects"]), "null") {
		t.Errorf("the columns or the projects are null: %s", body)
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestThePreferencesRoutesRefuseWhatIsNotAllowed(t *testing.T) {
	st := newStack(t)
	got := st.do(http.MethodPatch, "/v1/me/preferences", protocol.UpdatePreferencesRequest{Theme: ptr(protocol.Theme("sepia"))}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if got.Message != "That is not a theme Marshal knows. Choose light, dark, or system." {
		t.Errorf("message = %q", got.Message)
	}
	st.do(http.MethodPatch, "/v1/me/preferences", protocol.UpdatePreferencesRequest{
		Projects: map[string]protocol.UpdateProjectPreferences{"no-such-project": {Query: ptr("x")}},
	}).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPatch, "/v1/me/preferences", `{"theme":"dark","layout":"wide"}`).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if prefs := decode[protocol.Preferences](t, st.do(http.MethodGet, "/v1/me/preferences", nil).want(t, http.StatusOK)); prefs.Theme != protocol.ThemeSystem {
		t.Errorf("a refused change was kept: %+v", prefs)
	}
}

func TestTheProgressRoutes(t *testing.T) {
	st := newStack(t)
	r := st.do(http.MethodGet, "/v1/me/progress", nil).want(t, http.StatusOK)
	sameShape(t, "progress", r.Body)
	if got := decode[protocol.Progress](t, r); got.Onboarding.Status != protocol.ProgressStatusPending || got.Onboarding.Step != 0 {
		t.Errorf("a new install's progress = %+v", got)
	}

	// Onboarding is left on its third screen, and resumed after a restart.
	st.do(http.MethodPatch, "/v1/me/progress", protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Step: ptr(2)},
	}).want(t, http.StatusOK)
	st.restart()
	got := decode[protocol.Progress](t, st.do(http.MethodGet, "/v1/me/progress", nil).want(t, http.StatusOK))
	if got.Onboarding.Step != 2 || got.Onboarding.Status != protocol.ProgressStatusPending {
		t.Errorf("the progress after a restart = %+v", got)
	}

	// Skipping onboarding is told apart from finishing it; the tour can be finished and replayed.
	r = st.do(http.MethodPatch, "/v1/me/progress", protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Status: ptr(protocol.ProgressStatusSkipped)},
		Tutorial:   &protocol.UpdateTutorialProgress{Status: ptr(protocol.ProgressStatusDone)},
	}).want(t, http.StatusOK)
	sameShape(t, "progress", r.Body)
	got = decode[protocol.Progress](t, r)
	if got.Onboarding.Status != protocol.ProgressStatusSkipped || got.Tutorial.Status != protocol.ProgressStatusDone ||
		got.Onboarding.FinishedAt == nil || got.Tutorial.FinishedAt == nil {
		t.Errorf("the progress = %+v", got)
	}
	got = decode[protocol.Progress](t, st.do(http.MethodPatch, "/v1/me/progress", protocol.UpdateProgressRequest{
		Tutorial: &protocol.UpdateTutorialProgress{Status: ptr(protocol.ProgressStatusPending)},
	}).want(t, http.StatusOK))
	if got.Tutorial.Status != protocol.ProgressStatusPending || got.Tutorial.FinishedAt != nil {
		t.Errorf("a replayed tour = %+v", got.Tutorial)
	}

	refusal := st.do(http.MethodPatch, "/v1/me/progress", protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Step: ptr(10)},
	}).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if refusal.Message != "Onboarding has no screen 10. Use a screen from 0 to 9." {
		t.Errorf("message = %q", refusal.Message)
	}
}

func TestTheUsersRoute(t *testing.T) {
	st := newStack(t)
	r := st.do(http.MethodGet, "/v1/users", nil).want(t, http.StatusOK)
	sameShape(t, "user-list", r.Body)
	list := decode[protocol.UserListSnapshot](t, r)
	profile := decode[protocol.Profile](t, st.do(http.MethodGet, "/v1/me", nil).want(t, http.StatusOK))
	if len(list.Users) != 1 || list.Users[0].ID != profile.ID || list.Users[0].Name != profile.Name || list.Users[0].Initials != profile.Initials {
		t.Errorf("the users list = %+v, want the owner alone", list.Users)
	}
}

func TestTheAvatarRoutes(t *testing.T) {
	st := newStack(t)
	image := onePixelPNG(t)

	r := st.upload("image/png", image).want(t, http.StatusOK)
	sameShape(t, "profile", r.Body)
	profile := decode[protocol.Profile](t, r)
	if profile.AvatarURL == nil || !strings.HasPrefix(*profile.AvatarURL, "/v1/users/"+profile.ID+"/avatar?v=") {
		t.Fatalf("the avatar address = %v", profile.AvatarURL)
	}

	// The image is served byte for byte, as its own kind, and never as anything a browser would run.
	got := st.do(http.MethodGet, *profile.AvatarURL, nil).want(t, http.StatusOK)
	if !bytes.Equal(got.Body, image) {
		t.Errorf("the image served is %d bytes, want the %d uploaded", len(got.Body), len(image))
	}
	if got.Header.Get("Content-Type") != "image/png" || got.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers = %v, want image/png and nosniff", got.Header)
	}
	if got.Header.Get("Cache-Control") != "private, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want the current version to be kept for a year", got.Header.Get("Cache-Control"))
	}
	if got.Header.Get("Last-Modified") == "" {
		t.Error("the image has no Last-Modified, so a client cannot ask whether it changed")
	}
	// An address that names an older version, or none, is not kept: it is checked each time.
	stale := st.do(http.MethodGet, "/v1/users/"+profile.ID+"/avatar?v=1", nil).want(t, http.StatusOK)
	if stale.Header.Get("Cache-Control") != "private, no-cache" {
		t.Errorf("Cache-Control of an old address = %q", stale.Header.Get("Cache-Control"))
	}
	// Asking for a part of it works, as it does for any file.
	req := st.newRequest(http.MethodGet, *profile.AvatarURL, nil)
	req.Header.Set("Authorization", "Bearer "+st.token)
	req.Header.Set("Range", "bytes=0-3")
	if part := st.send(req).want(t, http.StatusPartialContent); !bytes.Equal(part.Body, image[:4]) {
		t.Errorf("a range of the image = %v, want its first four bytes", part.Body)
	}

	// The image is in the users list, and it needs the token like every other route.
	list := decode[protocol.UserListSnapshot](t, st.do(http.MethodGet, "/v1/users", nil).want(t, http.StatusOK))
	if list.Users[0].AvatarURL == nil || *list.Users[0].AvatarURL != *profile.AvatarURL {
		t.Errorf("the users list avatar = %v", list.Users[0].AvatarURL)
	}
	st.doWith("", http.MethodGet, *profile.AvatarURL, nil).apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)

	// It survives a restart, and removing it takes it away.
	st.restart()
	st.do(http.MethodGet, *profile.AvatarURL, nil).want(t, http.StatusOK)
	removed := decode[protocol.Profile](t, st.do(http.MethodDelete, "/v1/me/avatar", nil).want(t, http.StatusOK))
	if removed.AvatarURL != nil {
		t.Errorf("the profile after removing = %+v", removed)
	}
	st.do(http.MethodGet, *profile.AvatarURL, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	// Removing again is not an error.
	st.do(http.MethodDelete, "/v1/me/avatar", nil).want(t, http.StatusOK)
}

func TestTheAvatarRoutesRefuseWhatIsNotAnImage(t *testing.T) {
	st := newStack(t)
	image := onePixelPNG(t)
	const notAnImage = "Marshal accepts PNG, JPEG, and WebP images. Choose one of those."
	tests := []struct {
		name string
		kind string
		body []byte
		text string
	}{
		{"a JSON body", "application/json", []byte(`{}`), notAnImage},
		{"a GIF", "image/gif", []byte("GIF89a......"), notAnImage},
		{"a PNG that says it is a JPEG", "image/jpeg", image, notAnImage},
		{"a page that says it is a PNG", "image/png", []byte("<html></html>"), notAnImage},
		{"no kind", "", image, notAnImage},
		{"a kind with a parameter that is not one", "image/png; charset=", image, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := st.upload(tc.kind, tc.body)
			if tc.text == "" {
				// A parameter on the kind is fine: only the type is read.
				r.want(t, http.StatusOK)
				return
			}
			got := r.apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
			if got.Message != tc.text {
				t.Errorf("message = %q, want %q", got.Message, tc.text)
			}
		})
	}
	st.do(http.MethodPost, "/v1/me/avatar", nil).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}

func TestTheAvatarRouteRefusesAnImageThatIsTooLarge(t *testing.T) {
	st := newStack(t)
	big := append(onePixelPNG(t), make([]byte, accounts.MaxAvatarBytes)...)
	const tooLarge = "That image is larger than 2 MB. Choose a smaller one."

	// One that says how large it is, and one that does not, so its size is found by reading it.
	got := st.upload("image/png", big).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if got.Message != tooLarge {
		t.Errorf("message = %q, want %q", got.Message, tooLarge)
	}
	req := st.newRequest(http.MethodPost, "/v1/me/avatar", big)
	req.Header.Set("Content-Type", "image/png")
	req.Header.Set("Authorization", "Bearer "+st.token)
	req.ContentLength = -1
	got = st.send(req).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if got.Message != tooLarge {
		t.Errorf("message = %q, want %q", got.Message, tooLarge)
	}
	if profile := decode[protocol.Profile](t, st.do(http.MethodGet, "/v1/me", nil).want(t, http.StatusOK)); profile.AvatarURL != nil {
		t.Errorf("a refused image became the avatar: %v", *profile.AvatarURL)
	}
}

func TestTheAvatarRouteRefusesAnAddressThatIsNotAnAvatar(t *testing.T) {
	st := newStack(t)
	profile := decode[protocol.Profile](t, st.do(http.MethodGet, "/v1/me", nil).want(t, http.StatusOK))
	// A person with no avatar, a person who is not there, and an id that cannot be one.
	for _, path := range []string{
		"/v1/users/" + profile.ID + "/avatar", "/v1/users/01M3C107JB041061050R3GG28A/avatar", "/v1/users/not-an-id/avatar",
	} {
		st.do(http.MethodGet, path, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	}
	// The address of an avatar is for reading only.
	st.do(http.MethodPost, "/v1/users/"+profile.ID+"/avatar", nil).apiError(t, http.StatusMethodNotAllowed, protocol.ErrorCodeMethodNotAllowed)
}

// The dev reset puts first-launch progress back, and tells the other devices. It is an address only a
// dev daemon has.
func TestTheDevResetOfFirstLaunch(t *testing.T) {
	st := newStack(t)
	stream := st.connect(protocol.MeTopic)
	st.do(http.MethodPatch, "/v1/me/progress", protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Step: ptr(3), Status: ptr(protocol.ProgressStatusDone)},
		Tutorial:   &protocol.UpdateTutorialProgress{Status: ptr(protocol.ProgressStatusSkipped)},
	}).want(t, http.StatusOK)
	meOf(t, stream)

	r := st.do(http.MethodPost, "/v1/dev/reset-first-launch", nil).want(t, http.StatusOK)
	sameShape(t, "progress", r.Body)
	got := decode[protocol.Progress](t, r)
	if got.Onboarding.Status != protocol.ProgressStatusPending || got.Onboarding.Step != 0 || got.Tutorial.Status != protocol.ProgressStatusPending {
		t.Errorf("the progress after the reset = %+v", got)
	}
	if me := meOf(t, stream); me.Progress.Onboarding.Status != protocol.ProgressStatusPending {
		t.Errorf("the other device heard %+v", me.Progress)
	}
	// What the reset does not touch stays: the profile and the preferences.
	if profile := decode[protocol.Profile](t, st.do(http.MethodGet, "/v1/me", nil).want(t, http.StatusOK)); profile.Name != "Owner" {
		t.Errorf("the reset changed the profile: %+v", profile)
	}
}

func TestTheDevResetDoesNotExistOnANormalDaemon(t *testing.T) {
	st := newStack(t, normalDaemon())
	// A normal daemon has every other route of the person, so it is the address that is missing.
	st.do(http.MethodGet, "/v1/me", nil).want(t, http.StatusOK)
	got := st.do(http.MethodPost, "/v1/dev/reset-first-launch", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	if got.Message != nothingThereMsg {
		t.Errorf("message = %q, want %q", got.Message, nothingThereMsg)
	}
	st.do(http.MethodPost, "/v1/me/progress", nil).apiError(t, http.StatusMethodNotAllowed, protocol.ErrorCodeMethodNotAllowed)
	st.do(http.MethodPatch, "/v1/me/progress", protocol.UpdateProgressRequest{
		Onboarding: &protocol.UpdateOnboardingProgress{Step: ptr(2)},
	}).want(t, http.StatusOK)
	// Nothing reset it.
	if got := decode[protocol.Progress](t, st.do(http.MethodGet, "/v1/me/progress", nil).want(t, http.StatusOK)); got.Onboarding.Step != 2 {
		t.Errorf("the progress = %+v", got)
	}
}

// The `me` topic follows the rules of the other topics: it takes no id, and a device that was away
// hears what it missed, in order, when it comes back with the position it had reached.
func TestTheMeTopicFollowsTheRulesOfTheOtherTopics(t *testing.T) {
	st := newStack(t)

	refused := st.dialRaw()
	refused.sendRaw(websocket.MessageText, `{"type":"hello","subscribe":["me:01M3C107JB041061050R3GG2U1"]}`)
	refused.expectRefusal(protocol.ErrorCodeInvalidArgument)

	away := st.connect(protocol.MeTopic)
	epoch, seq := away.epoch, away.lastSeq
	_ = away.conn.CloseNow()
	st.do(http.MethodPatch, "/v1/me/preferences", protocol.UpdatePreferencesRequest{Theme: ptr(protocol.ThemeDark)}).want(t, http.StatusOK)
	name := "Ada Okafor"
	st.do(http.MethodPatch, "/v1/me", protocol.UpdateProfileRequest{Name: &name}).want(t, http.StatusOK)

	back := st.dialRaw()
	back.epoch, back.lastSeq = epoch, seq
	back.hello(protocol.MeTopic)
	if first := meOf(t, back); first.Preferences.Theme != protocol.ThemeDark || first.Profile.Name != "Owner" {
		t.Errorf("the first missed event = %+v, want the theme change alone", first)
	}
	if second := meOf(t, back); second.Profile.Name != name || second.Preferences.Theme != protocol.ThemeDark {
		t.Errorf("the second missed event = %+v, want the name change on top of the theme", second)
	}
	if len(back.resyncs) != 0 {
		t.Errorf("a device that missed two events was told to reload: %+v", back.resyncs)
	}
}

// The person's routes follow the accounts service, the saved view routes follow the projects service,
// and the dev reset follows the accounts service and the daemon's mode. This asks each stack about
// every one of them on its own, so it holds whatever else the route table grows.
func TestThePersonsRoutesAreRegisteredOnlyWhereTheyBelong(t *testing.T) {
	personRoutes := []string{
		"GET /v1/me", "PATCH /v1/me", "POST /v1/me/avatar", "DELETE /v1/me/avatar", "GET /v1/users",
		"GET /v1/users/{id}/avatar", "GET /v1/me/progress", "PATCH /v1/me/progress",
		"GET /v1/me/preferences", "PATCH /v1/me/preferences",
	}
	savedViewRoutes := []string{
		"GET /v1/projects/{id}/saved-views", "POST /v1/projects/{id}/saved-views",
		"PATCH /v1/saved-views/{id}", "DELETE /v1/saved-views/{id}",
	}
	devRoutes := []string{"POST /v1/dev/reset-first-launch"}
	known := append(append(append([]string{}, personRoutes...), savedViewRoutes...), devRoutes...)
	inTable := map[string]bool{}
	for _, pattern := range api.RoutePatterns() {
		inTable[pattern] = true
	}
	for _, pattern := range known {
		if !inTable[pattern] {
			t.Errorf("%s is not in the route table", pattern)
		}
	}
	registered := func(st *stack, pattern string) bool {
		method, path := concretePath(pattern)
		got := st.do(method, path, nil)
		missing := got.Status == http.StatusNotFound && strings.Contains(string(got.Body), nothingThereMsg)
		return got.Status != http.StatusMethodNotAllowed && !missing
	}
	tests := []struct {
		name                   string
		opts                   []stackOption
		person, saved, devOnly bool
	}{
		{"a dev daemon with everything", nil, true, true, true},
		{"a normal daemon", []stackOption{normalDaemon()}, true, true, false},
		{"no accounts service", []stackOption{withoutAccounts()}, false, true, false},
		{"no projects service", []stackOption{withoutProjects()}, true, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newStack(t, tc.opts...)
			for group, want := range map[string]struct {
				routes []string
				there  bool
			}{
				"the person's routes":   {personRoutes, tc.person},
				"the saved view routes": {savedViewRoutes, tc.saved},
				"the dev reset":         {devRoutes, tc.devOnly},
			} {
				for _, pattern := range want.routes {
					if got := registered(st, pattern); got != want.there {
						t.Errorf("%s (%s) is registered %v, want %v", pattern, group, got, want.there)
					}
				}
			}
		})
	}
}
