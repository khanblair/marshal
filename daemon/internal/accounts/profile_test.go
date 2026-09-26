package accounts_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/accounts"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A new install has one person, the owner, with a name and nothing else, and no avatar.
func TestProfileOfANewInstall(t *testing.T) {
	e := newEnv(t)
	profile, err := e.svc.Profile(context.Background(), e.userID)
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}
	if profile.ID != e.userID || profile.Name != "Owner" || profile.Initials != "O" || profile.Email != "" ||
		profile.TimeZone != "" || profile.AvatarURL != nil || profile.TailnetIdentity != "" {
		t.Errorf("profile = %+v", profile)
	}
	if profile.CreatedAt == (protocol.Timestamp{}) || profile.UpdatedAt == (protocol.Timestamp{}) {
		t.Errorf("the profile has no times: %+v", profile)
	}
}

// A profile change saves the fields it sets, keeps the rest, trims what it is given, and is
// published with the profile, the preferences, and the progress as they now are.
func TestUpdateProfile(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	name, email, zone := "  Ada Okafor  ", " ada@example.com ", "Europe/London"
	before, _ := e.svc.Profile(ctx, e.userID)

	got, err := e.svc.UpdateProfile(ctx, e.userID, protocol.UpdateProfileRequest{Name: &name, Email: &email, TimeZone: &zone})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if got.Name != "Ada Okafor" || got.Email != "ada@example.com" || got.TimeZone != "Europe/London" || got.Initials != "AO" {
		t.Errorf("profile = %+v", got)
	}
	if !got.UpdatedAt.Time().After(before.UpdatedAt.Time()) || got.CreatedAt != before.CreatedAt {
		t.Errorf("times = created %v updated %v, want the same creation and a later update than %v",
			got.CreatedAt.Time(), got.UpdatedAt.Time(), before.UpdatedAt.Time())
	}

	me := e.nextMe(t)
	if !reflect.DeepEqual(me.Profile, got) {
		t.Errorf("the event's profile = %+v, want %+v", me.Profile, got)
	}
	if me.Preferences.Theme != protocol.ThemeSystem || me.Progress.Onboarding.Status != protocol.ProgressStatusPending {
		t.Errorf("the event does not carry the preferences and the progress: %+v", me)
	}

	// Changing one field keeps the others.
	other := "Ada"
	again, err := e.svc.UpdateProfile(ctx, e.userID, protocol.UpdateProfileRequest{Name: &other})
	if err != nil || again.Email != "ada@example.com" || again.TimeZone != "Europe/London" || again.Name != "Ada" {
		t.Errorf("UpdateProfile of the name only = %+v, %v", again, err)
	}
	e.nextMe(t)

	// The empty string clears an email and a time zone.
	none := ""
	cleared, err := e.svc.UpdateProfile(ctx, e.userID, protocol.UpdateProfileRequest{Email: &none, TimeZone: &none})
	if err != nil || cleared.Email != "" || cleared.TimeZone != "" || cleared.Name != "Ada" {
		t.Errorf("UpdateProfile clearing = %+v, %v", cleared, err)
	}
	e.nextMe(t)
}

// A change that changes nothing answers with the profile and publishes nothing, whether the body is
// empty or names what is already there.
func TestUpdateProfileThatChangesNothing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	before, _ := e.svc.Profile(ctx, e.userID)
	same := "Owner"
	for name, in := range map[string]protocol.UpdateProfileRequest{
		"an empty body": {}, "the same name": {Name: &same}, "the same name with spaces": {Name: ptr(" Owner ")},
	} {
		got, err := e.svc.UpdateProfile(ctx, e.userID, in)
		if err != nil || !reflect.DeepEqual(got, before) {
			t.Errorf("%s: UpdateProfile = %+v, %v; want the profile unchanged", name, got, err)
		}
	}
	e.noEvent(t)
}

func ptr[T any](value T) *T { return &value }

// Initials are the first letter of each of the first two words, in capitals, as the profile form
// shows them while a name is typed.
func TestInitials(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	tests := map[string]string{
		"Ada Okafor":         "AO",
		"blair":              "B",
		"  ada   lovelace  ": "AL",
		"Ada Augusta King":   "AA",
		"élodie martin":      "ÉM",
	}
	for name, want := range tests {
		got, err := e.svc.UpdateProfile(ctx, e.userID, protocol.UpdateProfileRequest{Name: ptr(name)})
		if err != nil || got.Initials != want {
			t.Errorf("initials of %q = %q, %v; want %q", name, got.Initials, err, want)
		}
	}
}

// Each refusal is a plain sentence, and nothing is changed or published by it.
func TestUpdateProfileRefusals(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	const badEmail = "That does not look like an email address. Check it and try again."
	tests := []struct {
		name string
		in   protocol.UpdateProfileRequest
		text string
	}{
		{"no name", protocol.UpdateProfileRequest{Name: ptr("")}, "Enter a name. It shows on cards you comment on."},
		{"a name of spaces", protocol.UpdateProfileRequest{Name: ptr("   ")}, "Enter a name. It shows on cards you comment on."},
		{"a long name", protocol.UpdateProfileRequest{Name: ptr(strings.Repeat("x", 101))}, "A name can have at most 100 characters."},
		{"an email with no at sign", protocol.UpdateProfileRequest{Email: ptr("ada")}, badEmail},
		{"an email with a name in front", protocol.UpdateProfileRequest{Email: ptr("Ada <ada@example.com>")}, badEmail},
		{"two emails", protocol.UpdateProfileRequest{Email: ptr("a@example.com, b@example.com")}, badEmail},
		{"a long email", protocol.UpdateProfileRequest{Email: ptr(strings.Repeat("a", 250) + "@example.com")}, badEmail},
		{"a time zone that is not one", protocol.UpdateProfileRequest{TimeZone: ptr("Europe/Atlantis")},
			"Marshal does not know that time zone. Choose one from the list."},
		{"the machine's own zone", protocol.UpdateProfileRequest{TimeZone: ptr("Local")},
			"Marshal does not know that time zone. Choose one from the list."},
		{"a valid field beside a bad one", protocol.UpdateProfileRequest{Name: ptr("Ada"), Email: ptr("nope")}, badEmail},
	}
	before, _ := e.svc.Profile(ctx, e.userID)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.UpdateProfile(ctx, e.userID, tc.in)
			wantCode(t, err, protocol.ErrorCodeInvalidArgument, tc.text)
		})
	}
	after, _ := e.svc.Profile(ctx, e.userID)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("a refusal changed the profile: %+v, want %+v", after, before)
	}
	e.noEvent(t)
}

// A person who is not there is not found, by every call that takes one.
func TestAPersonWhoIsNotThereIsNotFound(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	const missing = "01M3C107JB041061050R3GG28A"
	const text = "Marshal cannot find that user. It may have been removed."
	name := "Ada"
	calls := map[string]func() error{
		"Profile": func() error { _, err := e.svc.Profile(ctx, missing); return err },
		"UpdateProfile": func() error {
			_, err := e.svc.UpdateProfile(ctx, missing, protocol.UpdateProfileRequest{Name: &name})
			return err
		},
		"Progress": func() error { _, err := e.svc.Progress(ctx, missing); return err },
		"UpdateProgress": func() error {
			_, err := e.svc.UpdateProgress(ctx, missing, protocol.UpdateProgressRequest{})
			return err
		},
		"ResetFirstLaunch": func() error { _, err := e.svc.ResetFirstLaunch(ctx, missing); return err },
		"Preferences":      func() error { _, err := e.svc.Preferences(ctx, missing); return err },
		"UpdatePreferences": func() error {
			_, err := e.svc.UpdatePreferences(ctx, missing, protocol.UpdatePreferencesRequest{})
			return err
		},
		"Me": func() error { _, err := e.svc.Me(ctx, missing); return err },
		"SetAvatar": func() error {
			_, err := e.svc.SetAvatar(ctx, missing, "image/png", strings.NewReader(pngBytes))
			return err
		},
		"RemoveAvatar": func() error { _, err := e.svc.RemoveAvatar(ctx, missing); return err },
		"Avatar":       func() error { _, err := e.svc.Avatar(ctx, missing); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			wantCode(t, call(), protocol.ErrorCodeNotFound, text)
		})
	}
	e.noEvent(t)
}

// The profile is in the file, so a daemon that starts again reads what was saved.
func TestTheProfileSurvivesARestart(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	saved, err := e.svc.UpdateProfile(ctx, e.userID, protocol.UpdateProfileRequest{
		Name: ptr("Ada Okafor"), Email: ptr("ada@example.com"), TimeZone: ptr("Africa/Lagos"),
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	e.restart(t)
	got, err := e.svc.Profile(ctx, e.userID)
	if err != nil || !reflect.DeepEqual(got, saved) {
		t.Errorf("after a restart the profile = %+v, %v; want %+v", got, err, saved)
	}
}

// The users list has the owner, by name, with initials and no email or time zone, and lists a second
// person and their avatar the same way when there is one.
func TestUsersListsEveryoneByName(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	solo, err := e.svc.Users(ctx)
	if err != nil || len(solo.Users) != 1 || solo.Users[0].ID != e.userID || solo.Users[0].Initials != "O" ||
		solo.Users[0].AvatarURL != nil || solo.ServerTime == (protocol.Timestamp{}) {
		t.Fatalf("Users of a new install = %+v, %v", solo, err)
	}
	second := e.secondUser(t, "Ada Okafor")
	if _, err := e.svc.SetAvatar(ctx, e.userID, "image/png", strings.NewReader(pngBytes)); err != nil {
		t.Fatalf("SetAvatar: %v", err)
	}
	list, err := e.svc.Users(ctx)
	if err != nil || len(list.Users) != 2 {
		t.Fatalf("Users = %+v, %v", list, err)
	}
	if list.Users[0].ID != second || list.Users[0].Name != "Ada Okafor" || list.Users[0].Initials != "AO" || list.Users[0].AvatarURL != nil {
		t.Errorf("first user = %+v, want Ada Okafor, who has no avatar", list.Users[0])
	}
	if list.Users[1].ID != e.userID || list.Users[1].AvatarURL == nil {
		t.Errorf("second user = %+v, want the owner, who has an avatar", list.Users[1])
	}
}

// The service is not built without what it needs: every change is published, and a preference that
// named a project the service could not check would be a guess.
func TestNewRefusesWhatItCannotWorkWithout(t *testing.T) {
	e := newEnv(t)
	good := accounts.Deps{Store: e.store, Bus: e.bus, Projects: e.projects, DataDir: e.dataDir}
	tests := map[string]accounts.Deps{
		"no store":                              {Bus: e.bus, Projects: e.projects, DataDir: e.dataDir},
		"no bus":                                {Store: e.store, Projects: e.projects, DataDir: e.dataDir},
		"no projects":                           {Store: e.store, Bus: e.bus, DataDir: e.dataDir},
		"a data folder that is not a full path": {Store: e.store, Bus: e.bus, Projects: e.projects, DataDir: "data"},
	}
	for name, deps := range tests {
		if _, err := accounts.New(deps); err == nil {
			t.Errorf("New with %s built a service", name)
		}
	}
	// A clock and a logger that are nil leave the defaults, and do not stop the service working.
	svc, err := accounts.New(good, accounts.WithClock(nil), accounts.WithLogger(nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := svc.Profile(context.Background(), e.userID); err != nil {
		t.Errorf("Profile: %v", err)
	}
}
