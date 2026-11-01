package integrations_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

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
