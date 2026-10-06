package integrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/store/db"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

const (
	scopeBase   = "https://www.googleapis.com/auth/"
	driveScope  = scopeBase + "drive.file"
	docsScope   = scopeBase + "documents.readonly"
	sheetsScope = scopeBase + "spreadsheets.readonly"
	slidesScope = scopeBase + "presentations.readonly"
)

// grantedScopes is what Google reports for each connection when nothing was unticked.
var grantedScopes = map[string]string{
	integrations.GCalID:    scopeBase + "calendar.readonly",
	integrations.GmailID:   scopeBase + "gmail.readonly",
	integrations.GDriveID:  driveScope,
	integrations.GDocsID:   docsScope + " " + driveScope,
	integrations.GSheetsID: sheetsScope + " " + driveScope,
	integrations.GSlidesID: slidesScope + " " + driveScope,
}

var fileIDs = []string{integrations.GDriveID, integrations.GDocsID, integrations.GSheetsID, integrations.GSlidesID}

// newFilesFixture is a Google fixture whose Drive, Docs, Sheets and Slides are one fake.
func newFilesFixture(t *testing.T, mutate ...func(*integrations.Options)) (*fixture, *fakeGoogle, *testutil.GoogleFiles) {
	t.Helper()
	files := testutil.NewGoogleFiles(t)
	opts := append([]func(*integrations.Options){func(o *integrations.Options) { o.GoogleFilesBaseURL = files.URL }}, mutate...)
	f, g := newGoogleFixture(t, opts...)
	return f, g, files
}

// connect runs the whole consent flow of one Google connection, with every scope granted.
func (f *fixture) connect(g *fakeGoogle, id string) {
	f.t.Helper()
	g.scope.Store(grantedScopes[id])
	if got := f.grant(func(ctx context.Context) (string, error) { return f.svc.AuthorizeGoogle(ctx, id) }); got != id {
		f.t.Fatalf("granted %q, want %q", got, id)
	}
}

// listed is one connection's row as the settings screen reads it.
func (f *fixture) listed(id string) protocol.Integration {
	f.t.Helper()
	list, err := f.svc.List(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	for _, row := range list {
		if row.ID == id {
			return row
		}
	}
	f.t.Fatalf("no %s row", id)
	return protocol.Integration{}
}

func checkNames(result protocol.TestResult) string {
	var names []string
	for _, check := range result.Checks {
		names = append(names, check.Name)
	}
	return strings.Join(names, ",")
}

func TestConsentForEachFileConnectionAsksForItsOwnScopesAndNothingMore(t *testing.T) {
	f, _, _ := newFilesFixture(t)
	want := map[string]string{
		integrations.GDriveID:  driveScope,
		integrations.GDocsID:   docsScope + " " + driveScope,
		integrations.GSheetsID: sheetsScope + " " + driveScope,
		integrations.GSlidesID: slidesScope + " " + driveScope,
	}
	for id, scope := range want {
		raw, err := f.svc.AuthorizeGoogle(context.Background(), id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if got := mustQuery(t, raw).Get("scope"); got != scope {
			t.Errorf("%s asked for %q, want %q", id, got, scope)
		}
	}
}

func TestConsentForEachFileConnectionStoresItsOwnTokenAndMakesItsRow(t *testing.T) {
	for _, id := range fileIDs {
		t.Run(id, func(t *testing.T) {
			f, g, _ := newBundledFilesFixture(t)
			if got := f.listed(id); got.Status != protocol.IntegrationStatusNone {
				t.Fatalf("before the grant the row reads %q", got.Status)
			}
			f.connect(g, id)
			config, keychainRef, found := f.row(id)
			wantConfig := "{}"
			if id == integrations.GDriveID {
				wantConfig = `{"folder":"Marshal"}`
			}
			if !found || config != wantConfig || keychainRef != id {
				t.Errorf("row = %q %q found %v, want %q filed under %q", config, keychainRef, found, wantConfig, id)
			}
			if got := f.listed(id); got.Status != protocol.IntegrationStatusConnected || got.Kind != kindOf(id) {
				t.Errorf("after the grant the row reads %q kind %q", got.Status, got.Kind)
			}
			raw, err := f.keys.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			var stored map[string]json.RawMessage
			if err := json.Unmarshal([]byte(raw), &stored); err != nil || len(stored) != 1 || stored["token"] == nil {
				t.Errorf("secret = %q, want the token alone, as Gmail keeps it", raw)
			}
			// A Drive grant must not make Calendar read connected: it holds no token of its own.
			if _, _, calendar := f.row(integrations.GCalID); calendar {
				t.Error("granting a file connection made a Google Calendar row")
			}
			for _, other := range fileIDs {
				if other != id {
					if _, err := f.keys.Get(other); !errors.Is(err, security.ErrNoKey) {
						t.Errorf("%s's grant stored something for %s: %v", id, other, err)
					}
				}
			}
		})
	}
}

func kindOf(id string) string {
	return map[string]string{
		integrations.GDriveID: "drive", integrations.GDocsID: "docs", integrations.GSheetsID: "sheets", integrations.GSlidesID: "slides",
	}[id]
}

// newBundledFilesFixture is a daemon with Marshal's own Google client and no row saved anywhere.
func newBundledFilesFixture(t *testing.T) (*fixture, *fakeGoogle, *testutil.GoogleFiles) {
	t.Helper()
	g := newFakeGoogle(t)
	files := testutil.NewGoogleFiles(t)
	f := newFixture(t, func(o *integrations.Options) {
		o.GCalRedirectURL = "http://127.0.0.1:47801/v1/integrations/gcal/callback"
		o.GoogleAuthURL = g.server.URL + "/auth"
		o.GoogleTokenURL = g.server.URL + "/token"
		o.GoogleRevokeURL = g.server.URL + "/revoke"
		o.GoogleFilesBaseURL = files.URL
		o.GoogleClientID, o.GoogleClientSecret = "marshal-client-id", "marshal-client-secret"
	})
	return f, g, files
}

func TestAGrantWithABoxUntickedIsRefusedAndStoresNothing(t *testing.T) {
	for _, tc := range []struct{ id, granted string }{
		{integrations.GDocsID, driveScope},
		{integrations.GDocsID, docsScope},
		{integrations.GSheetsID, sheetsScope},
		{integrations.GSlidesID, slidesScope},
		{integrations.GDriveID, scopeBase + "calendar.readonly"},
	} {
		f, g, _ := newBundledFilesFixture(t)
		g.scope.Store(tc.granted)
		raw, err := f.svc.AuthorizeGoogle(context.Background(), tc.id)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.svc.FinishGoogle(context.Background(), "the-code", mustQuery(t, raw).Get("state"))
		var refusal *protocol.Error
		if !errors.As(err, &refusal) || refusal.Message != "Google did not give Marshal every permission it asked for. Connect again and leave every box ticked." {
			t.Errorf("%s with %q: err = %v, want the unticked-box sentence", tc.id, tc.granted, err)
		}
		if _, err := f.keys.Get(tc.id); !errors.Is(err, security.ErrNoKey) {
			t.Errorf("%s: a token was stored for a partial grant: %v", tc.id, err)
		}
		if _, _, found := f.row(tc.id); found {
			t.Errorf("%s: a row was made for a partial grant", tc.id)
		}
		if g.revokeCalls.Load() != 0 {
			t.Errorf("%s: the partial grant was revoked, which would end every scope Marshal holds", tc.id)
		}
	}
}

func TestMoreScopesThanAskedForAreNotARefusal(t *testing.T) {
	f, g, _ := newBundledFilesFixture(t)
	g.scope.Store(grantedScopes[integrations.GDocsID] + " https://www.googleapis.com/auth/userinfo.email")
	f.grant(func(ctx context.Context) (string, error) { return f.svc.AuthorizeGoogle(ctx, integrations.GDocsID) })
	if _, err := f.keys.Get(integrations.GDocsID); err != nil {
		t.Errorf("a grant with an extra scope was refused: %v", err)
	}
}

func TestOnlyAGoogleConnectionCanStartAConsentFlow(t *testing.T) {
	f, _, _ := newFilesFixture(t)
	for _, id := range []string{"github", "telegram", "obsidian", "nope", ""} {
		_, err := f.svc.AuthorizeGoogle(context.Background(), id)
		var refusal *protocol.Error
		if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeNotFound {
			t.Errorf("AuthorizeGoogle(%q) = %v, want not found", id, err)
		}
	}
	// Not found comes before "no client", so a daemon with no client says the same for a bad id.
	bare, _ := newGoogleFixtureWithoutClient(t)
	_, err := bare.svc.AuthorizeGoogle(context.Background(), "github")
	var refusal *protocol.Error
	if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeNotFound {
		t.Errorf("with no client, a bad id gave %v, want not found", err)
	}
}

func TestDisconnectingOneGoogleConnectionNeverRevokesWhileAnotherHoldsAToken(t *testing.T) {
	f, g, _ := newFilesFixture(t)
	f.connect(g, integrations.GCalID)
	f.connect(g, integrations.GDriveID)
	if err := f.svc.Remove(context.Background(), integrations.GDriveID); err != nil {
		t.Fatal(err)
	}
	if g.revokeCalls.Load() != 0 {
		t.Fatal("Drive was removed while Calendar held a token, and the access was revoked at Google")
	}
	if _, err := f.svc.GoogleCalendarClient(context.Background()); err != nil {
		t.Errorf("Calendar stopped working after Drive was removed: %v", err)
	}
	if _, err := f.keys.Get(integrations.GDriveID); !errors.Is(err, security.ErrNoKey) {
		t.Errorf("Drive's token stayed: %v", err)
	}
	if _, _, found := f.row(integrations.GDriveID); found {
		t.Error("Drive's row stayed")
	}
	if err := f.svc.Remove(context.Background(), integrations.GCalID); err != nil {
		t.Fatal(err)
	}
	if g.revokeCalls.Load() != 1 || g.lastRevoked.Load() != "refresh-1" {
		t.Errorf("revoke calls = %d, token = %v, want one call once the last connection went", g.revokeCalls.Load(), g.lastRevoked.Load())
	}
}

func TestTheLastOfSeveralFileConnectionsRevokesAndTheOthersDoNot(t *testing.T) {
	f, g, _ := newFilesFixture(t)
	for _, id := range fileIDs {
		f.connect(g, id)
	}
	for i, id := range fileIDs {
		if err := f.svc.Remove(context.Background(), id); err != nil {
			t.Fatal(err)
		}
		wantRevokes := int32(0)
		if i == len(fileIDs)-1 {
			wantRevokes = 1
		}
		if g.revokeCalls.Load() != wantRevokes {
			t.Fatalf("after removing %s: %d revoke calls, want %d", id, g.revokeCalls.Load(), wantRevokes)
		}
	}
}

func TestATokenThatCannotBeReadCountsAsHeldSoNothingIsRevoked(t *testing.T) {
	f, g, _ := newFilesFixture(t)
	f.connect(g, integrations.GDriveID)
	if err := f.keys.Set(integrations.GDocsID, "this is not json"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Remove(context.Background(), integrations.GDriveID); err != nil {
		t.Fatal(err)
	}
	if g.revokeCalls.Load() != 0 {
		t.Error("Docs' token could not be read, and Drive's removal revoked the access anyway")
	}
}

func TestChangingTheClientDropsEveryFileConnectionsTokenAndKeepsTheirRows(t *testing.T) {
	f, g, _ := newFilesFixture(t)
	for _, id := range append([]string{integrations.GmailID}, fileIDs...) {
		f.connect(g, id)
	}
	if err := f.svc.SaveGoogleCalendar(context.Background(), protocol.SaveGoogleCalendarRequest{ClientID: "another", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range append([]string{integrations.GmailID}, fileIDs...) {
		if _, err := f.keys.Get(id); !errors.Is(err, security.ErrNoKey) {
			t.Errorf("%s's token, granted to the old client, stayed: %v", id, err)
		}
	}
	got := f.listed(integrations.GDocsID)
	if got.Status != protocol.IntegrationStatusNone || got.Detail != "Grant access to finish connecting Google Docs." {
		t.Errorf("Docs reads %q %q, want it asking for a new grant", got.Status, got.Detail)
	}
}

func TestARowWithNoTokenReadsAsNeedingTheGrantAndNeverShowsAnOldTest(t *testing.T) {
	f, _, _ := newFilesFixture(t)
	if err := f.svc.SaveGoogleDrive(context.Background(), protocol.SaveGoogleDriveRequest{Folder: "Team files"}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Write(context.Background(), func(q *db.Queries) error {
		return q.SetIntegrationTest(context.Background(), db.SetIntegrationTestParams{
			ID: integrations.GDriveID, Kind: "drive", LastTestAt: time.Now().UnixMilli(),
			LastTestResultJSON: `{"connectionId":"gdrive","checks":[],"ok":true,"ranAt":"2026-10-05T10:00:00.000Z"}`,
		})
	}); err != nil {
		t.Fatal(err)
	}
	got := f.listed(integrations.GDriveID)
	if got.Status != protocol.IntegrationStatusNone || got.Detail != "Grant access to finish connecting Google Drive." || got.LastTest != nil {
		t.Errorf("Drive with a folder and no grant reads %q %q last test %v", got.Status, got.Detail, got.LastTest)
	}
}

func TestATestRunBeforeAnyGrantLeavesARowThatTheGrantStillFills(t *testing.T) {
	f, g, _ := newBundledFilesFixture(t)
	// Pressing Test first saves a row with no settings and no keychain name.
	if err := f.store.Write(context.Background(), func(q *db.Queries) error {
		return q.SetIntegrationTest(context.Background(), db.SetIntegrationTestParams{
			ID: integrations.GDocsID, Kind: "docs", LastTestAt: time.Now().UnixMilli(),
			LastTestResultJSON: `{"connectionId":"gdocs","checks":[],"ok":true,"ranAt":"2026-10-05T10:00:00.000Z"}`,
		})
	}); err != nil {
		t.Fatal(err)
	}
	if config, ref, found := f.row(integrations.GDocsID); !found || config != "" || ref != "" {
		t.Fatalf("setup: row = %q %q %v", config, ref, found)
	}
	f.connect(g, integrations.GDocsID)
	config, ref, _ := f.row(integrations.GDocsID)
	if config != "{}" || ref != integrations.GDocsID {
		t.Errorf("row after the grant = %q %q, want {} filed under gdocs", config, ref)
	}
	if got := f.listed(integrations.GDocsID); got.Status != protocol.IntegrationStatusConnected {
		t.Errorf("Docs reads %q after the grant, want connected", got.Status)
	}
}

func TestAGrantNeverReplacesTheFolderAPersonSaved(t *testing.T) {
	f, g, _ := newBundledFilesFixture(t)
	if err := f.svc.SaveGoogleDrive(context.Background(), protocol.SaveGoogleDriveRequest{Folder: "  Team files  "}); err != nil {
		t.Fatal(err)
	}
	f.connect(g, integrations.GDriveID)
	if config, _, _ := f.row(integrations.GDriveID); config != `{"folder":"Team files"}` {
		t.Errorf("config = %q, want the saved folder, trimmed and kept", config)
	}
}

func TestTheFolderNameIsHeldToOneToAHundredCharactersAndNoHiddenOnes(t *testing.T) {
	f, _, _ := newFilesFixture(t)
	for _, bad := range []string{"", "   ", strings.Repeat("a", 101), "line\nbreak", "tab\there", "bell\a"} {
		err := f.svc.SaveGoogleDrive(context.Background(), protocol.SaveGoogleDriveRequest{Folder: bad})
		var refusal *protocol.Error
		if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeInvalidArgument {
			t.Errorf("folder %q: err = %v, want a refusal", bad, err)
		}
	}
	for _, good := range []string{"a", strings.Repeat("é", 100), "Mom's \\ files"} {
		if err := f.svc.SaveGoogleDrive(context.Background(), protocol.SaveGoogleDriveRequest{Folder: good}); err != nil {
			t.Errorf("folder %q refused: %v", good, err)
		}
	}
}

func TestEachFileConnectionsTestPassesAgainstTheFakeWithItsOwnSentence(t *testing.T) {
	wants := map[string]struct{ checks, summary string }{
		integrations.GDriveID:  {"Summary,Access,Drive,Folder", "Google Drive works. Files go in the folder “Marshal”."},
		integrations.GDocsID:   {"Summary,Access,Drive,Docs", "Google Docs works. Marshal makes documents in the folder “Marshal” and reads any you share by link."},
		integrations.GSheetsID: {"Summary,Access,Drive,Sheets", "Google Sheets works. Marshal makes spreadsheets in the folder “Marshal” and reads any you share by link."},
		integrations.GSlidesID: {"Summary,Access,Drive,Slides", "Google Slides works. Marshal makes presentations in the folder “Marshal” and reads any you share by link."},
	}
	for id, want := range wants {
		f, g, files := newFilesFixture(t)
		f.connect(g, id)
		result, err := f.svc.Test(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		summary := result.Checks[0]
		if !result.OK || checkNames(result) != want.checks || summary.Message != want.summary || summary.State != protocol.CheckStatePassed {
			t.Errorf("%s: ok %v checks %s summary %q", id, result.OK, checkNames(result), summary.Message)
		}
		if made := files.Made(); len(made) != 0 {
			t.Errorf("%s: the test made %d files, want none", id, len(made))
		}
		for _, req := range files.Requests() {
			if req.Method != http.MethodGet {
				t.Errorf("%s: the test sent %s %s, want reads only", id, req.Method, req.Path)
			}
		}
	}
}

func TestTheDriveTestSaysWhetherTheFolderIsThere(t *testing.T) {
	f, g, files := newFilesFixture(t)
	f.connect(g, integrations.GDriveID)
	folder := func() string {
		result, err := f.svc.Test(context.Background(), integrations.GDriveID)
		if err != nil {
			t.Fatal(err)
		}
		for _, check := range result.Checks {
			if check.Name == integrations.CheckFolder {
				return string(check.State) + ": " + check.Message
			}
		}
		return "no folder check"
	}
	if got := folder(); got != "passed: Marshal makes the folder “Marshal” when it saves the first file." {
		t.Errorf("no folder yet: %q", got)
	}
	files.SetFolder("folder-9")
	if got := folder(); got != "passed: The folder “Marshal” is there." {
		t.Errorf("folder there: %q", got)
	}
	if err := f.svc.SaveGoogleDrive(context.Background(), protocol.SaveGoogleDriveRequest{Folder: "Team files"}); err != nil {
		t.Fatal(err)
	}
	files.SetFolder("")
	if got := folder(); got != "passed: Marshal makes the folder “Team files” when it saves the first file." {
		t.Errorf("named folder: %q", got)
	}
}

func failedSummary(t *testing.T, result protocol.TestResult) protocol.TestCheck {
	t.Helper()
	if result.OK || len(result.Checks) == 0 || result.Checks[0].Name != integrations.CheckSummary || result.Checks[0].State != protocol.CheckStateFailed {
		t.Fatalf("result = %+v, want a failed summary first", result)
	}
	return result.Checks[0]
}

func TestATestSaysWhichAPIIsTurnedOffAndHowToTurnItOn(t *testing.T) {
	tests := []struct{ id, api, check, name string }{
		{integrations.GDriveID, "drive", "Drive", "Google Drive"},
		{integrations.GDocsID, "drive", "Drive", "Google Drive"},
		{integrations.GDocsID, "docs", "Docs", "Google Docs"},
		{integrations.GSheetsID, "sheets", "Sheets", "Google Sheets"},
		{integrations.GSlidesID, "slides", "Slides", "Google Slides"},
	}
	for _, tc := range tests {
		f, g, files := newFilesFixture(t)
		f.connect(g, tc.id)
		files.TurnOff(tc.api)
		result, err := f.svc.Test(context.Background(), tc.id)
		if err != nil {
			t.Fatal(err)
		}
		summary := failedSummary(t, result)
		wantFix := "Turn on the " + tc.name + " API in the Google Cloud project Marshal signs in with."
		if summary.Fix != wantFix {
			t.Errorf("%s with %s off: summary fix = %q, want %q", tc.id, tc.api, summary.Fix, wantFix)
		}
		var named *protocol.TestCheck
		for i := range result.Checks {
			if result.Checks[i].Name == tc.check && result.Checks[i].State == protocol.CheckStateFailed {
				named = &result.Checks[i]
			}
		}
		if named == nil || named.Fix != wantFix {
			t.Errorf("%s with %s off: the %s check = %+v", tc.id, tc.api, tc.check, named)
		}
		if got := f.listed(tc.id); got.Status != protocol.IntegrationStatusConnected {
			t.Errorf("a row with no saved test reads %q", got.Status)
		}
	}
}

func TestATestWithARevokedTokenAsksToReconnectByName(t *testing.T) {
	for _, id := range fileIDs {
		f, g, _ := newFilesFixture(t)
		f.connect(g, id)
		g.invalidGrant.Store(true)
		result, err := f.svc.Test(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		name := integrations.GoogleName(id)
		summary := failedSummary(t, result)
		if len(result.Checks) != 1 || summary.Message != "Google no longer accepts Marshal's access to "+name+"." || summary.Fix != "Reconnect "+name+" in Settings." {
			t.Errorf("%s: %+v", id, result.Checks)
		}
	}
}

func TestATestWhereGoogleRefusesTheTokenOnTheAPIAsksToReconnectToo(t *testing.T) {
	f, g, files := newFilesFixture(t)
	f.connect(g, integrations.GDocsID)
	files.Unauthorized.Store(true)
	result, err := f.svc.Test(context.Background(), integrations.GDocsID)
	if err != nil {
		t.Fatal(err)
	}
	summary := failedSummary(t, result)
	if summary.Message != "Google no longer accepts Marshal's access to Google Docs." || summary.Fix != "Reconnect Google Docs in Settings." {
		t.Errorf("summary = %+v", summary)
	}
}

func TestATestBeforeTheGrantSaysTheConnectionIsNotConnected(t *testing.T) {
	f, _, _ := newFilesFixture(t)
	for _, id := range fileIDs {
		result, err := f.svc.Test(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		summary := failedSummary(t, result)
		if len(result.Checks) != 1 || summary.Message != integrations.GoogleName(id)+" is not connected." ||
			summary.Fix != "Choose Connect with Google in Settings, under Integrations." {
			t.Errorf("%s: %+v", id, result.Checks)
		}
	}
}

func TestATestSaysGoogleCouldNotBeReachedWhenNothingAnswers(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	f, g, _ := newFilesFixture(t, func(o *integrations.Options) { o.GoogleFilesBaseURL = dead.URL })
	f.connect(g, integrations.GDocsID)
	result, err := f.svc.Test(context.Background(), integrations.GDocsID)
	if err != nil {
		t.Fatal(err)
	}
	summary := failedSummary(t, result)
	if summary.Message != "Marshal could not reach Google Drive." || summary.Fix != "Check this computer's connection, then test again." {
		t.Errorf("summary = %+v", summary)
	}
}

func TestMakingAFileNeedsItsConnectionAndNamesItWhenMissing(t *testing.T) {
	f, g, files := newFilesFixture(t)
	f.connect(g, integrations.GSheetsID)
	checks := map[string]func() error{
		integrations.GDocsID: func() error {
			_, err := f.svc.CreateGoogleDoc(context.Background(), protocol.CreateGoogleDocRequest{Title: "T", Text: "x"})
			return err
		},
		integrations.GSlidesID: func() error {
			_, err := f.svc.CreateGoogleSlides(context.Background(), protocol.CreateGoogleSlidesRequest{Title: "T", Slides: []protocol.GoogleSlide{{Title: "a"}}})
			return err
		},
		integrations.GDriveID: func() error {
			_, err := f.svc.UploadGoogleFile(context.Background(), protocol.UploadGoogleFileRequest{Name: "a.txt", Content: "x"})
			return err
		},
	}
	for id, run := range checks {
		var access *integrations.GoogleAccessError
		err := run()
		if !errors.As(err, &access) || access.ID != id || !errors.Is(err, integrations.ErrNotConnected) {
			t.Errorf("%s: err = %v, want a not-connected error that names it", id, err)
		}
	}
	if len(files.Requests()) != 0 {
		t.Errorf("%d requests reached Google for connections that are not connected", len(files.Requests()))
	}
	g.invalidGrant.Store(true)
	_, err := f.svc.CreateGoogleSheet(context.Background(), protocol.CreateGoogleSheetRequest{Title: "T", Rows: [][]string{{"a"}}})
	var access *integrations.GoogleAccessError
	if !errors.As(err, &access) || access.ID != integrations.GSheetsID || !errors.Is(err, integrations.ErrNeedsReconnect) {
		t.Errorf("a revoked token: err = %v, want a reconnect error that names Sheets", err)
	}
}

func TestAFileIsMadeInMarshalsFolderWhichIsMadeOnceAndAnsweredWithARealLink(t *testing.T) {
	f, g, files := newFilesFixture(t)
	f.connect(g, integrations.GDocsID)
	f.connect(g, integrations.GSheetsID)
	f.connect(g, integrations.GSlidesID)
	f.connect(g, integrations.GDriveID)
	ctx := context.Background()

	doc, err := f.svc.CreateGoogleDoc(ctx, protocol.CreateGoogleDocRequest{Title: " Plan ", HTML: "<h1>Hi</h1>"})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "Plan" || doc.Kind != protocol.GoogleFileKindDoc || !strings.HasPrefix(doc.URL, "https://drive.google.com/open?id=") || doc.ModifiedAt == nil {
		t.Errorf("doc = %+v", doc)
	}
	sheet, err := f.svc.CreateGoogleSheet(ctx, protocol.CreateGoogleSheetRequest{Title: "Board", Rows: [][]string{{"Title", "Note"}, {"=1+1", "ok"}}})
	if err != nil || sheet.Kind != protocol.GoogleFileKindSheet {
		t.Fatalf("sheet = %+v, %v", sheet, err)
	}
	deck, err := f.svc.CreateGoogleSlides(ctx, protocol.CreateGoogleSlidesRequest{Title: "Deck", Slides: []protocol.GoogleSlide{{Title: "One", Bullets: []string{"a"}}}})
	if err != nil || deck.Kind != protocol.GoogleFileKindSlides {
		t.Fatalf("deck = %+v, %v", deck, err)
	}
	plain, err := f.svc.UploadGoogleFile(ctx, protocol.UploadGoogleFileRequest{Name: "notes.md", Content: "# hi", MimeType: "text/markdown"})
	if err != nil || plain.Kind != protocol.GoogleFileKindFile {
		t.Fatalf("file = %+v, %v", plain, err)
	}

	var folders, made int
	var folderID string
	for _, file := range files.Made() {
		if file.MimeType == "application/vnd.google-apps.folder" {
			folders++
			folderID = file.ID
			if file.Name != "Marshal" {
				t.Errorf("folder named %q, want Marshal", file.Name)
			}
			continue
		}
		made++
		if len(file.Parents) != 1 || file.Parents[0] != folderID {
			t.Errorf("%s was made in %v, want Marshal's folder %s", file.Name, file.Parents, folderID)
		}
	}
	if folders != 1 || made != 4 {
		t.Errorf("%d folders and %d files made, want 1 and 4: every connection shares one folder", folders, made)
	}
	byName := map[string]testutil.GoogleFile{}
	for _, file := range files.Made() {
		byName[file.Name] = file
	}
	if got := byName["Plan"]; got.MediaType != "text/html; charset=utf-8" || got.Media != "<h1>Hi</h1>" || got.MimeType != "application/vnd.google-apps.document" {
		t.Errorf("doc upload = %+v", got)
	}
	if got := byName["Board"]; got.Media != "Title,Note\n'=1+1,ok\n" || got.MimeType != "application/vnd.google-apps.spreadsheet" {
		t.Errorf("sheet upload = %+v, want the formula kept as text", got)
	}
	if got := byName["notes.md"]; got.MediaType != "text/markdown" || got.MimeType != "text/markdown" {
		t.Errorf("file upload = %+v, want its own type with no conversion", got)
	}
}

func TestAllFourConnectionsUseTheFolderSavedUnderDriveWhetherOrNotDriveIsConnected(t *testing.T) {
	f, g, files := newFilesFixture(t)
	f.connect(g, integrations.GDocsID)
	if err := f.svc.SaveGoogleDrive(context.Background(), protocol.SaveGoogleDriveRequest{Folder: "Team files"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateGoogleDoc(context.Background(), protocol.CreateGoogleDocRequest{Title: "T", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	var searched string
	for _, req := range files.Requests() {
		if req.Path == "/drive/v3/files" && req.Method == http.MethodGet && strings.Contains(req.Query.Get("q"), "folder") {
			searched = req.Query.Get("q")
		}
	}
	if !strings.Contains(searched, "name='Team files'") {
		t.Errorf("folder search = %q, want the saved name", searched)
	}
	if made := files.Made(); made[0].Name != "Team files" {
		t.Errorf("folder made = %q", made[0].Name)
	}
	list, err := f.svc.GoogleFiles(context.Background(), protocol.GoogleFileKindDoc)
	if err != nil || list.Folder != "Team files" {
		t.Errorf("list = %+v, %v, want the saved folder named", list, err)
	}
}

func TestAFolderThatAlreadyExistsIsUsedAndOneDeletedLaterIsFoundAgain(t *testing.T) {
	f, g, files := newFilesFixture(t)
	f.connect(g, integrations.GDocsID)
	files.SetFolder("existing-folder")
	doc, err := f.svc.CreateGoogleDoc(context.Background(), protocol.CreateGoogleDocRequest{Title: "A", Text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	made := files.Made()
	if len(made) != 1 || made[0].Parents[0] != "existing-folder" || doc.ID != made[0].ID {
		t.Fatalf("made = %+v, want one file in the folder that was already there", made)
	}
	// The person deletes the folder in Drive. The remembered id is now wrong.
	files.DeleteFolder()
	if _, err := f.svc.CreateGoogleDoc(context.Background(), protocol.CreateGoogleDocRequest{Title: "B", Text: "x"}); err != nil {
		t.Fatalf("a deleted folder stopped the next save: %v", err)
	}
	made = files.Made()
	if len(made) != 3 || made[1].Name != "Marshal" || made[2].Parents[0] != made[1].ID {
		t.Errorf("made = %+v, want a new folder and the file inside it", made)
	}
}

func TestWhatMarshalWillNotMakeIsRefusedInPlainWordsBeforeGoogleIsAsked(t *testing.T) {
	f, g, files := newFilesFixture(t)
	for _, id := range fileIDs {
		f.connect(g, id)
	}
	ctx := context.Background()
	tooLong := strings.Repeat("a", 201)
	big := strings.Repeat("a", 2<<20+1)
	rows := func(n, columns int) [][]string {
		out := make([][]string, n)
		for i := range out {
			out[i] = make([]string, columns)
		}
		return out
	}
	slides := func(n, bullets int) []protocol.GoogleSlide {
		out := make([]protocol.GoogleSlide, n)
		for i := range out {
			out[i] = protocol.GoogleSlide{Title: "t", Bullets: make([]string, bullets)}
		}
		return out
	}
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{"doc with no title", func() error {
			_, err := f.svc.CreateGoogleDoc(ctx, protocol.CreateGoogleDocRequest{Title: "  ", Text: "x"})
			return err
		}, "The title must be 1 to 200 characters."},
		{"doc with a long title", func() error {
			_, err := f.svc.CreateGoogleDoc(ctx, protocol.CreateGoogleDocRequest{Title: tooLong, Text: "x"})
			return err
		}, "The title must be 1 to 200 characters."},
		{"doc with both", func() error {
			_, err := f.svc.CreateGoogleDoc(ctx, protocol.CreateGoogleDocRequest{Title: "T", HTML: "<p>", Text: "x"})
			return err
		}, "Send the document as HTML or as plain text, not both."},
		{"doc with neither", func() error {
			_, err := f.svc.CreateGoogleDoc(ctx, protocol.CreateGoogleDocRequest{Title: "T"})
			return err
		}, "Send the document's words as HTML or as plain text."},
		{"doc over 2 MB", func() error {
			_, err := f.svc.CreateGoogleDoc(ctx, protocol.CreateGoogleDocRequest{Title: "T", HTML: big})
			return err
		}, "That document is too big. Marshal saves up to 2 MB at a time."},
		{"sheet with no rows", func() error {
			_, err := f.svc.CreateGoogleSheet(ctx, protocol.CreateGoogleSheetRequest{Title: "T"})
			return err
		}, "Add at least one row to the sheet."},
		{"sheet with too many rows", func() error {
			_, err := f.svc.CreateGoogleSheet(ctx, protocol.CreateGoogleSheetRequest{Title: "T", Rows: rows(10_001, 1)})
			return err
		}, "A sheet can have up to 10,000 rows."},
		{"sheet with too many columns", func() error {
			_, err := f.svc.CreateGoogleSheet(ctx, protocol.CreateGoogleSheetRequest{Title: "T", Rows: rows(1, 101)})
			return err
		}, "A sheet can have up to 100 columns."},
		{"sheet over 2 MB", func() error {
			_, err := f.svc.CreateGoogleSheet(ctx, protocol.CreateGoogleSheetRequest{Title: "T", Rows: [][]string{{big}}})
			return err
		}, "That sheet is too big. Marshal saves up to 2 MB at a time."},
		{"no slides", func() error {
			_, err := f.svc.CreateGoogleSlides(ctx, protocol.CreateGoogleSlidesRequest{Title: "T"})
			return err
		}, "A presentation needs 1 to 100 slides."},
		{"too many slides", func() error {
			_, err := f.svc.CreateGoogleSlides(ctx, protocol.CreateGoogleSlidesRequest{Title: "T", Slides: slides(101, 0)})
			return err
		}, "A presentation needs 1 to 100 slides."},
		{"too many bullets", func() error {
			_, err := f.svc.CreateGoogleSlides(ctx, protocol.CreateGoogleSlidesRequest{Title: "T", Slides: slides(1, 51)})
			return err
		}, "A slide can have up to 50 lines under its title."},
		{"file with no name", func() error {
			_, err := f.svc.UploadGoogleFile(ctx, protocol.UploadGoogleFileRequest{Name: "", Content: "x"})
			return err
		}, "The name must be 1 to 200 characters."},
		{"file over 2 MB", func() error {
			_, err := f.svc.UploadGoogleFile(ctx, protocol.UploadGoogleFileRequest{Name: "a", Content: big})
			return err
		}, "That file is too big. Marshal saves up to 2 MB at a time."},
		{"file that would be converted", func() error {
			_, err := f.svc.UploadGoogleFile(ctx, protocol.UploadGoogleFileRequest{Name: "a", Content: "x", MimeType: "application/vnd.google-apps.document"})
			return err
		}, "Marshal cannot save a file of that type. Use a type like text/plain or text/markdown."},
		{"file with a type that injects a header", func() error {
			_, err := f.svc.UploadGoogleFile(ctx, protocol.UploadGoogleFileRequest{Name: "a", Content: "x", MimeType: "text/plain\r\nX-Evil: 1"})
			return err
		}, "Marshal cannot save a file of that type. Use a type like text/plain or text/markdown."},
	}
	for _, tc := range tests {
		var refusal *protocol.Error
		err := tc.run()
		if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeInvalidArgument || refusal.Message != tc.want {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
	for _, req := range files.Requests() {
		t.Errorf("a refused request still reached Google: %s %s", req.Method, req.Path)
	}
	// Right at each limit is allowed.
	if _, err := f.svc.CreateGoogleSheet(ctx, protocol.CreateGoogleSheetRequest{Title: strings.Repeat("a", 200), Rows: rows(10_000, 1)}); err != nil {
		t.Errorf("a sheet with the most rows was refused: %v", err)
	}
	if _, err := f.svc.CreateGoogleSheet(ctx, protocol.CreateGoogleSheetRequest{Title: "T", Rows: rows(1, 100)}); err != nil {
		t.Errorf("a sheet with the most columns was refused: %v", err)
	}
	if _, err := f.svc.CreateGoogleSlides(ctx, protocol.CreateGoogleSlidesRequest{Title: "T", Slides: slides(100, 50)}); err != nil {
		t.Errorf("a presentation at the limits was refused: %v", err)
	}
}

func TestAPlainFileDefaultsToPlainText(t *testing.T) {
	f, g, files := newFilesFixture(t)
	f.connect(g, integrations.GDriveID)
	if _, err := f.svc.UploadGoogleFile(context.Background(), protocol.UploadGoogleFileRequest{Name: "a.txt", Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	made := files.Made()
	if got := made[len(made)-1]; got.MediaType != "text/plain; charset=utf-8" || got.Media != "hello" {
		t.Errorf("upload = %+v", got)
	}
}

func TestFilesAreListedByKindWithTheMatchingConnectionAndNeverAsNull(t *testing.T) {
	f, g, _ := newFilesFixture(t)
	f.connect(g, integrations.GDocsID)
	ctx := context.Background()
	empty, err := f.svc.GoogleFiles(ctx, protocol.GoogleFileKindDoc)
	if err != nil || empty.Files == nil || len(empty.Files) != 0 || empty.Folder != "Marshal" {
		t.Fatalf("an empty list = %+v, %v", empty, err)
	}
	for _, title := range []string{"first", "second"} {
		if _, err := f.svc.CreateGoogleDoc(ctx, protocol.CreateGoogleDocRequest{Title: title, Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := f.svc.GoogleFiles(ctx, protocol.GoogleFileKindDoc)
	if err != nil || len(list.Files) != 2 || list.Files[0].Name != "second" || list.Files[0].Kind != protocol.GoogleFileKindDoc {
		t.Errorf("list = %+v, %v, want newest first", list, err)
	}
	for _, kind := range []protocol.GoogleFileKind{protocol.GoogleFileKindSheet, protocol.GoogleFileKindSlides, protocol.GoogleFileKindFile, ""} {
		_, err := f.svc.GoogleFiles(ctx, kind)
		var access *integrations.GoogleAccessError
		wantID := map[protocol.GoogleFileKind]string{
			protocol.GoogleFileKindSheet: integrations.GSheetsID, protocol.GoogleFileKindSlides: integrations.GSlidesID,
			protocol.GoogleFileKindFile: integrations.GDriveID, "": integrations.GDriveID,
		}[kind]
		if !errors.As(err, &access) || access.ID != wantID {
			t.Errorf("kind %q with only Docs connected: err = %v, want %s named", kind, err, wantID)
		}
	}
	_, err = f.svc.GoogleFiles(ctx, "movie")
	var refusal *protocol.Error
	if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeInvalidArgument {
		t.Errorf("a kind that does not exist gave %v", err)
	}
}

func TestListingWithNoKindUsesDriveAndEveryKindButFolders(t *testing.T) {
	f, g, files := newFilesFixture(t)
	f.connect(g, integrations.GDriveID)
	if _, err := f.svc.UploadGoogleFile(context.Background(), protocol.UploadGoogleFileRequest{Name: "a.txt", Content: "x"}); err != nil {
		t.Fatal(err)
	}
	list, err := f.svc.GoogleFiles(context.Background(), "")
	if err != nil || len(list.Files) != 1 || list.Files[0].Name != "a.txt" {
		t.Fatalf("list = %+v, %v, want the file and not the folder", list, err)
	}
	reqs := files.Requests()
	if q := reqs[len(reqs)-1].Query.Get("q"); q != "trashed=false and mimeType!='application/vnd.google-apps.folder'" {
		t.Errorf("list asked q=%q", q)
	}
}

const (
	docJSON = `{"title":"Launch plan","body":{"content":[
		{"paragraph":{"paragraphStyle":{"namedStyleType":"HEADING_1"},"elements":[{"textRun":{"content":"Launch plan\n"}}]}},
		{"paragraph":{"bullet":{"nestingLevel":0},"elements":[{"textRun":{"content":"Ship it\n","textStyle":{"bold":true}}}]}}]}}`
	sheetMeta   = `{"properties":{"title":"Budget"},"sheets":[{"properties":{"title":"Tab one"}}]}`
	sheetValues = `{"values":[["Item","Cost"],["Tea","3"]]}`
	deckJSON    = `{"title":"Roadmap","slides":[{"pageElements":[
		{"shape":{"placeholder":{"type":"TITLE"},"text":{"textElements":[{"textRun":{"content":"Goals\n"}}]}}},
		{"shape":{"placeholder":{"type":"BODY"},"text":{"textElements":[{"textRun":{"content":"Ship it\n"}}]}}}]}]}`
)

func TestALinkIsReadWithTheMatchingConnectionAndOnlyItsIdIsUsed(t *testing.T) {
	f, g, files := newFilesFixture(t)
	for _, id := range []string{integrations.GDocsID, integrations.GSheetsID, integrations.GSlidesID} {
		f.connect(g, id)
	}
	files.SetDoc("DOC1", docJSON)
	files.SetSheet("SHEET1", sheetMeta, sheetValues)
	files.SetDeck("DECK1", deckJSON)
	ctx := context.Background()
	tests := []struct {
		link, title, markdown string
		kind                  protocol.GoogleFileKind
		url                   string
	}{
		{"https://docs.google.com/document/d/DOC1/edit?usp=sharing", "Launch plan", "# Launch plan\n\n- **Ship it**", protocol.GoogleFileKindDoc,
			"https://docs.google.com/document/d/DOC1/edit"},
		{"https://docs.google.com/spreadsheets/d/SHEET1/edit#gid=0", "Budget", "| Item | Cost |\n| --- | --- |\n| Tea | 3 |", protocol.GoogleFileKindSheet,
			"https://docs.google.com/spreadsheets/d/SHEET1/edit"},
		{"https://docs.google.com/presentation/d/DECK1/edit", "Roadmap", "## 1. Goals\n- Ship it", protocol.GoogleFileKindSlides,
			"https://docs.google.com/presentation/d/DECK1/edit"},
	}
	for _, tc := range tests {
		got, err := f.svc.ReadGoogleLink(ctx, protocol.ReadGoogleLinkRequest{URL: tc.link})
		if err != nil {
			t.Fatalf("%s: %v", tc.link, err)
		}
		if got.Kind != tc.kind || got.Title != tc.title || got.Markdown != tc.markdown || got.URL != tc.url || got.Truncated {
			t.Errorf("%s: got %+v", tc.link, got)
		}
	}
	for _, req := range files.Requests() {
		if req.Method != http.MethodGet || !strings.HasPrefix(req.Path, "/v") {
			t.Errorf("a read sent %s %s", req.Method, req.Path)
		}
	}
}

func TestALinkThatIsNotAGoogleFileIsRefusedWithoutAskingGoogle(t *testing.T) {
	f, g, files := newFilesFixture(t)
	f.connect(g, integrations.GDocsID)
	for _, link := range []string{"", "hello", "https://example.com/document/d/DOC1/edit", "https://docs.google.com.evil.example/document/d/DOC1/edit",
		"http://127.0.0.1:8080/document/d/DOC1", "https://drive.google.com/file/d/DOC1/view"} {
		_, err := f.svc.ReadGoogleLink(context.Background(), protocol.ReadGoogleLinkRequest{URL: link})
		var refusal *protocol.Error
		if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeInvalidArgument ||
			refusal.Message != "Paste the address of a Google Doc, Sheet or Slides presentation." {
			t.Errorf("link %q: err = %v", link, err)
		}
	}
	if len(files.Requests()) != 0 {
		t.Errorf("%d requests reached anything for links that are not Google files", len(files.Requests()))
	}
}

func TestReadingALinkNeedsTheMatchingConnectionAndSaysWhenTheFileCannotBeOpened(t *testing.T) {
	f, g, files := newFilesFixture(t)
	f.connect(g, integrations.GDocsID)
	ctx := context.Background()
	_, err := f.svc.ReadGoogleLink(ctx, protocol.ReadGoogleLinkRequest{URL: "https://docs.google.com/spreadsheets/d/SHEET1/edit"})
	var access *integrations.GoogleAccessError
	if !errors.As(err, &access) || access.ID != integrations.GSheetsID || !errors.Is(err, integrations.ErrNotConnected) {
		t.Errorf("a sheet with only Docs connected: err = %v", err)
	}
	_, err = f.svc.ReadGoogleLink(ctx, protocol.ReadGoogleLinkRequest{URL: "https://docs.google.com/document/d/MISSING/edit"})
	var refusal *protocol.Error
	if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeRefused ||
		refusal.Message != "Marshal could not open that file. Check the link, and that the Google account you connected can open it." {
		t.Errorf("a file that is not there: err = %v", err)
	}
	files.TurnOff("docs")
	_, err = f.svc.ReadGoogleLink(ctx, protocol.ReadGoogleLinkRequest{URL: "https://docs.google.com/document/d/DOC1/edit"})
	if !errors.As(err, &refusal) || refusal.Message != "Turn on the Google Docs API in the Google Cloud project Marshal signs in with." {
		t.Errorf("the Docs API off: err = %v", err)
	}
	files.Unauthorized.Store(true)
	_, err = f.svc.ReadGoogleLink(ctx, protocol.ReadGoogleLinkRequest{URL: "https://docs.google.com/document/d/DOC1/edit"})
	if !errors.As(err, &access) || access.ID != integrations.GDocsID || !errors.Is(err, integrations.ErrNeedsReconnect) {
		t.Errorf("a token Google refuses: err = %v", err)
	}
}

func TestACachedGrantIsNotSharedAfterAReconnect(t *testing.T) {
	// The folder is remembered per grant and name. A new grant asks Google again.
	f, g, files := newFilesFixture(t)
	f.connect(g, integrations.GDocsID)
	for i := 0; i < 2; i++ {
		if _, err := f.svc.CreateGoogleDoc(context.Background(), protocol.CreateGoogleDocRequest{Title: "T", Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	searches := func() int {
		n := 0
		for _, req := range files.Requests() {
			if req.Method == http.MethodGet && req.Path == "/drive/v3/files" && strings.Contains(req.Query.Get("q"), "folder") {
				n++
			}
		}
		return n
	}
	if searches() != 1 {
		t.Fatalf("%d folder searches for two saves, want 1", searches())
	}
	f.connect(g, integrations.GDocsID)
	if _, err := f.svc.CreateGoogleDoc(context.Background(), protocol.CreateGoogleDocRequest{Title: "T", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if searches() != 2 {
		t.Errorf("%d folder searches after a reconnect, want 2", searches())
	}
}

func TestGmailAndCalendarKeepTheirOwnSecretsBesideTheFileConnections(t *testing.T) {
	f, g, _ := newFilesFixture(t)
	f.connect(g, integrations.GCalID)
	f.connect(g, integrations.GmailID)
	f.connect(g, integrations.GDocsID)
	raw, err := f.keys.Get(integrations.GCalID)
	if err != nil {
		t.Fatal(err)
	}
	var calendar struct {
		ClientSecret string        `json:"clientSecret"`
		Token        *oauth2.Token `json:"token"`
	}
	if err := json.Unmarshal([]byte(raw), &calendar); err != nil || calendar.ClientSecret != "client-secret" || calendar.Token == nil {
		t.Errorf("calendar secret = %q, want its client secret and token", raw)
	}
	if _, err := f.svc.GmailClient(context.Background()); err != nil {
		t.Errorf("Gmail: %v", err)
	}
	if _, err := f.svc.GoogleCalendarClient(context.Background()); err != nil {
		t.Errorf("Calendar: %v", err)
	}
}

func TestAGoogleThatCannotBeReachedIsAPlainSentenceAndNotAnInternalError(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	f, g, _ := newFilesFixture(t, func(o *integrations.Options) { o.GoogleFilesBaseURL = dead.URL })
	f.connect(g, integrations.GDocsID)
	ctx := context.Background()
	want := "Marshal could not reach Google. Check this computer's connection, then try again."
	_, err := f.svc.CreateGoogleDoc(ctx, protocol.CreateGoogleDocRequest{Title: "T", Text: "x"})
	var refusal *protocol.Error
	if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeUnavailable || refusal.Message != want {
		t.Errorf("making a doc: err = %v", err)
	}
	_, err = f.svc.GoogleFiles(ctx, protocol.GoogleFileKindDoc)
	if !errors.As(err, &refusal) || refusal.Message != want {
		t.Errorf("listing: err = %v", err)
	}
	_, err = f.svc.ReadGoogleLink(ctx, protocol.ReadGoogleLinkRequest{URL: "https://docs.google.com/document/d/DOC1/edit"})
	if !errors.As(err, &refusal) || refusal.Message != want {
		t.Errorf("reading: err = %v", err)
	}
}
