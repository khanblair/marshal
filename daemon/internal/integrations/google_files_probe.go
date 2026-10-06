package integrations

// This file is the connection test of the four Google file connections. It is read-only: it makes no
// file and no folder. It asks whether Marshal's access still works, whether the Drive API is on, and,
// for Docs, Sheets and Slides, whether that API is on too, by asking it for a file that cannot exist.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/integrations/googlefiles"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fileTest is what differs between the four tests: the API asked about beyond Drive, the name of its
// check, and the sentence a test that found nothing wrong ends with.
type fileTest struct {
	api   googlefiles.API
	check string
	works string
}

// fileTestOf is one file connection's test. The sentence has the folder's name in it.
func fileTestOf(id string) (fileTest, bool) {
	switch id {
	case GDriveID:
		return fileTest{works: "Google Drive works. Files go in the folder “%s”."}, true
	case GDocsID:
		return fileTest{googlefiles.APIDocs, CheckDocs,
			"Google Docs works. Marshal makes documents in the folder “%s” and reads any you share by link."}, true
	case GSheetsID:
		return fileTest{googlefiles.APISheets, CheckSheets,
			"Google Sheets works. Marshal makes spreadsheets in the folder “%s” and reads any you share by link."}, true
	case GSlidesID:
		return fileTest{googlefiles.APISlides, CheckSlides,
			"Google Slides works. Marshal makes presentations in the folder “%s” and reads any you share by link."}, true
	}
	return fileTest{}, false
}

func (s *Service) testGoogleFiles(ctx context.Context, info Info) (protocol.TestResult, error) {
	plan, ok := fileTestOf(info.ID)
	if !ok {
		return protocol.TestResult{}, protocol.NotFound("connection").With("id", info.ID)
	}
	name := GoogleName(info.ID)
	client, _, err := s.filesClient(ctx, info.ID)
	switch {
	case errors.Is(err, ErrNotConnected) || errors.Is(err, ErrNoGoogleClient):
		return s.failedSummary(info, name+" is not connected.", "Choose Connect with Google in Settings, under Integrations."), nil
	case errors.Is(err, ErrNeedsReconnect):
		return s.failedSummary(info, "Google no longer accepts Marshal's access to "+name+".", "Reconnect "+name+" in Settings."), nil
	case err != nil:
		return protocol.TestResult{}, err
	}
	folder, err := s.driveFolder(ctx)
	if err != nil {
		return protocol.TestResult{}, err
	}
	checks := []protocol.TestCheck{
		{Name: CheckAccess, State: protocol.CheckStatePassed, Message: "Google accepts Marshal's access."},
	}
	driveErr := client.ProbeDrive(ctx)
	drive := apiCheck(CheckDrive, googlefiles.APIDrive, name, driveErr)
	checks = append(checks, drive)
	if plan.api != "" {
		checks = append(checks, apiCheck(plan.check, plan.api, name, client.ProbeAPI(ctx, plan.api)))
	}
	if info.ID == GDriveID && driveErr == nil {
		checks = append(checks, folderCheck(ctx, client, folder))
	}
	works := fmt.Sprintf(plan.works, folder)
	checks = append([]protocol.TestCheck{summaryCheck(checks, works, works)}, checks...)
	return protocol.NewTestResult(info.ID, checks, s.now()), nil
}

// failedSummary is a test that could not go further: one failed check, which is also the row's sentence.
func (s *Service) failedSummary(info Info, message, fix string) protocol.TestResult {
	return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
		Name: CheckSummary, State: protocol.CheckStateFailed, Message: message, Fix: fix,
	}}, s.now())
}

// apiCheck is what one API answered to the test's call. A refusal of the token means the connection
// has to be made again, an API that is off says how to turn it on, and a Google that did not answer
// is a failure to reach it.
func apiCheck(name string, api googlefiles.API, connection string, err error) protocol.TestCheck {
	title := apiTitle(api)
	check := protocol.TestCheck{Name: name, State: protocol.CheckStateFailed}
	var apiErr *googlefiles.APIError
	switch {
	case err == nil:
		check.State = protocol.CheckStatePassed
		check.Message = title + " answers."
	case errors.As(err, &apiErr) && apiErr.Unauthorized():
		check.Message = "Google no longer accepts Marshal's access to " + connection + "."
		check.Fix = "Reconnect " + connection + " in Settings."
	case errors.As(err, &apiErr) && apiErr.NotEnabled():
		check.Message = "The " + title + " API is turned off."
		check.Fix = turnOnSentence(api)
	case errors.As(err, &apiErr) && apiErr.Status < http.StatusInternalServerError:
		check.Message = "Google would not let Marshal use " + title + "."
		check.Fix = "Reconnect " + connection + " in Settings, and leave every box ticked."
	default:
		check.Message = "Marshal could not reach " + title + "."
		check.Fix = "Check this computer's connection, then test again."
	}
	return check
}

// folderCheck says whether Marshal's folder is there. A folder that is not there yet is not a
// problem: Marshal makes it when it saves the first file.
func folderCheck(ctx context.Context, client *googlefiles.Client, folder string) protocol.TestCheck {
	id, err := client.FindFolder(ctx, folder)
	switch {
	case err != nil:
		check := apiCheck(CheckFolder, googlefiles.APIDrive, GoogleName(GDriveID), err)
		check.Message = fmt.Sprintf("Marshal could not look for the folder “%s”.", folder)
		return check
	case id == "":
		return protocol.TestCheck{
			Name: CheckFolder, State: protocol.CheckStatePassed,
			Message: fmt.Sprintf("Marshal makes the folder “%s” when it saves the first file.", folder),
		}
	}
	return protocol.TestCheck{
		Name: CheckFolder, State: protocol.CheckStatePassed, Message: fmt.Sprintf("The folder “%s” is there.", folder),
	}
}
