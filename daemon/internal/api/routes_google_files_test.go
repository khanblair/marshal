package api_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

const googleScopeBase = "https://www.googleapis.com/auth/"

// googleFileConnections are the four connections, with their names and what Google reports granted.
var googleFileConnections = []struct{ id, name, scope string }{
	{"gdrive", "Google Drive", googleScopeBase + "drive.file"},
	{"gdocs", "Google Docs", googleScopeBase + "documents.readonly " + googleScopeBase + "drive.file"},
	{"gsheets", "Google Sheets", googleScopeBase + "spreadsheets.readonly " + googleScopeBase + "drive.file"},
	{"gslides", "Google Slides", googleScopeBase + "presentations.readonly " + googleScopeBase + "drive.file"},
}

// newGoogleFilesStack is a daemon with Marshal's own Google client, pointed at a fake Google and a fake
// Drive, Docs, Sheets and Slides.
func newGoogleFilesStack(t *testing.T, more ...stackOption) (*stack, googleFake, *testutil.GoogleFiles) {
	t.Helper()
	files := testutil.NewGoogleFiles(t)
	fake := fakeGoogleServer(t, nil)
	fake.files = files.URL
	return newStack(t, append([]stackOption{withMarshalsGoogleClient(fake)}, more...)...), fake, files
}

// consentThroughTheAPI asks for one connection's consent address and then does what the browser does
// when Google sends the owner back: a visit with no token. It answers the callback's reply.
func consentThroughTheAPI(t *testing.T, st *stack, fake googleFake, id, granted string) reply {
	t.Helper()
	fake.scope.Store(granted)
	consent := decode[protocol.AuthorizeURL](t, st.do(http.MethodGet, "/v1/integrations/"+id+"/authorize", nil).want(t, http.StatusOK))
	u, err := url.Parse(consent.URL)
	if err != nil {
		t.Fatal(err)
	}
	return st.doWith("", http.MethodGet, "/v1/integrations/gcal/callback?code=the-code&state="+u.Query().Get("state"), nil)
}

func connectGoogleFile(t *testing.T, st *stack, fake googleFake, id string) {
	t.Helper()
	for _, conn := range googleFileConnections {
		if conn.id != id {
			continue
		}
		r := consentThroughTheAPI(t, st, fake, id, conn.scope)
		if r.Status != http.StatusOK || string(r.Body) != conn.name+" is connected. You can close this tab." {
			t.Fatalf("the callback = %d %q", r.Status, r.Body)
		}
		return
	}
	t.Fatalf("no connection %s", id)
}

func integrationRow(t *testing.T, st *stack, id string) protocol.Integration {
	t.Helper()
	list := decode[protocol.IntegrationList](t, st.do(http.MethodGet, "/v1/integrations", nil).want(t, http.StatusOK))
	for _, row := range list.Integrations {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("no %s row", id)
	return protocol.Integration{}
}

func TestGoogleFileConnectionsConsentThroughTheCallbackAndTestWithoutMakingAnything(t *testing.T) {
	for _, conn := range googleFileConnections {
		t.Run(conn.id, func(t *testing.T) {
			st, fake, files := newGoogleFilesStack(t)
			if row := integrationRow(t, st, conn.id); row.Status != protocol.IntegrationStatusNone {
				t.Fatalf("before the grant: %q", row.Status)
			}
			connectGoogleFile(t, st, fake, conn.id)
			row := integrationRow(t, st, conn.id)
			if row.Status != protocol.IntegrationStatusConnected || row.LastTest == nil || !row.LastTest.OK {
				t.Fatalf("after the grant the row = %+v, want connected with a passing test already run", row)
			}
			if !strings.HasPrefix(row.Detail, conn.name+" works.") {
				t.Errorf("detail = %q", row.Detail)
			}
			st.advance(time.Minute) // the grant ran a test, and another waits out the cooldown
			result := decode[protocol.TestResult](t, st.do(http.MethodPost, "/v1/integrations/"+conn.id+"/test", nil).want(t, http.StatusOK))
			if !result.OK {
				t.Errorf("test = %+v", result)
			}
			if len(files.Made()) != 0 {
				t.Errorf("the test made %d files", len(files.Made()))
			}
		})
	}
}

func TestGoogleAuthorizeIsNotFoundForAnyOtherConnection(t *testing.T) {
	st, _, _ := newGoogleFilesStack(t)
	for _, id := range []string{"github", "trello", "telegram", "obsidian", "nope"} {
		st.do(http.MethodGet, "/v1/integrations/"+id+"/authorize", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	}
	for _, id := range []string{"gcal", "gmail"} {
		st.do(http.MethodGet, "/v1/integrations/"+id+"/authorize", nil).want(t, http.StatusOK)
	}
	bare := newStack(t, withGoogle(fakeGoogleServer(t, nil)))
	bare.do(http.MethodGet, "/v1/integrations/gdocs/authorize", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

func TestGoogleCallbackSaysPlainlyWhenABoxWasUnticked(t *testing.T) {
	st, fake, _ := newGoogleFilesStack(t)
	r := consentThroughTheAPI(t, st, fake, "gdocs", googleScopeBase+"drive.file")
	want := "Google could not be connected: Google did not give Marshal every permission it asked for. Connect again and leave every box ticked."
	if r.Status != http.StatusBadRequest || string(r.Body) != want {
		t.Errorf("callback = %d %q, want %q", r.Status, r.Body, want)
	}
	if row := integrationRow(t, st, "gdocs"); row.Status != protocol.IntegrationStatusNone {
		t.Errorf("the row reads %q after a refused grant", row.Status)
	}
}

func TestGoogleDriveSavesItsFolderThroughTheIntegrationRouteAndTheOthersHaveNothingToSave(t *testing.T) {
	st, _, _ := newGoogleFilesStack(t)
	list := decode[protocol.IntegrationList](t, st.do(http.MethodPut, "/v1/integrations/gdrive", protocol.SaveGoogleDriveRequest{Folder: "Team files"}).
		want(t, http.StatusOK))
	var drive protocol.Integration
	for _, row := range list.Integrations {
		if row.ID == "gdrive" {
			drive = row
		}
	}
	if drive.Status != protocol.IntegrationStatusNone || drive.Detail != "Grant access to finish connecting Google Drive." || drive.LastTest != nil {
		t.Errorf("Drive with a folder and no grant = %+v", drive)
	}
	for _, id := range []string{"gdocs", "gsheets", "gslides"} {
		st.do(http.MethodPut, "/v1/integrations/"+id, `{}`).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	}
	got := st.do(http.MethodPut, "/v1/integrations/gdrive", protocol.SaveGoogleDriveRequest{Folder: strings.Repeat("a", 101)}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if got.Message != "The folder name must be 1 to 100 characters, with no line breaks or other hidden characters." {
		t.Errorf("message = %q", got.Message)
	}
	st.do(http.MethodPut, "/v1/integrations/gdrive", `{"folder":"x","extra":1}`).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}

func TestGoogleFilesAreMadeListedAndReadThroughEveryRoute(t *testing.T) {
	st, fake, files := newGoogleFilesStack(t)
	for _, conn := range googleFileConnections {
		connectGoogleFile(t, st, fake, conn.id)
	}
	files.SetDoc("DOC1", `{"title":"Notes","body":{"content":[{"paragraph":{"paragraphStyle":{"namedStyleType":"HEADING_2"},"elements":[{"textRun":{"content":"Hello\n"}}]}}]}}`)
	files.SetSheet("SHEET1", `{"properties":{"title":"Budget"},"sheets":[{"properties":{"title":"Sheet1"}}]}`, `{"values":[["a","b"],["1","2"]]}`)
	files.SetDeck("DECK1", `{"title":"Deck","slides":[{"pageElements":[{"shape":{"placeholder":{"type":"TITLE"},"text":{"textElements":[{"textRun":{"content":"Hi\n"}}]}}}]}]}`)

	doc := decode[protocol.GoogleFile](t, st.do(http.MethodPost, "/v1/google/docs", protocol.CreateGoogleDocRequest{Title: "Plan", HTML: "<h1>Plan</h1>"}).
		want(t, http.StatusCreated))
	sheet := decode[protocol.GoogleFile](t, st.do(http.MethodPost, "/v1/google/sheets", protocol.CreateGoogleSheetRequest{
		Title: "Board", Rows: [][]string{{"Title"}, {"=1+1"}},
	}).want(t, http.StatusCreated))
	deck := decode[protocol.GoogleFile](t, st.do(http.MethodPost, "/v1/google/slides", protocol.CreateGoogleSlidesRequest{
		Title: "Deck", Slides: []protocol.GoogleSlide{{Title: "One", Bullets: []string{"a", "b"}}},
	}).want(t, http.StatusCreated))
	plain := decode[protocol.GoogleFile](t, st.do(http.MethodPost, "/v1/google/drive/files", protocol.UploadGoogleFileRequest{
		Name: "notes.md", Content: "# hi", MimeType: "text/markdown",
	}).want(t, http.StatusCreated))
	if doc.Kind != protocol.GoogleFileKindDoc || sheet.Kind != protocol.GoogleFileKindSheet ||
		deck.Kind != protocol.GoogleFileKindSlides || plain.Kind != protocol.GoogleFileKindFile {
		t.Errorf("kinds = %q %q %q %q", doc.Kind, sheet.Kind, deck.Kind, plain.Kind)
	}
	for _, file := range []protocol.GoogleFile{doc, sheet, deck, plain} {
		if file.ID == "" || !strings.HasPrefix(file.URL, "https://") {
			t.Errorf("file = %+v, want an id and a real link", file)
		}
	}
	for _, made := range files.Made() {
		if made.MimeType != "application/vnd.google-apps.folder" && (len(made.Parents) != 1 || !strings.HasPrefix(made.Parents[0], "folder-")) {
			t.Errorf("%s was made in %v, want Marshal's folder", made.Name, made.Parents)
		}
	}
	if media := files.Made()[2].Media; media != "Title\n'=1+1\n" {
		t.Errorf("the sheet's cells = %q, want the formula kept as text", media)
	}

	for kind, want := range map[string]string{"doc": "Plan", "sheet": "Board", "slides": "Deck", "file": "notes.md"} {
		list := decode[protocol.GoogleFiles](t, st.do(http.MethodGet, "/v1/google/files?kind="+kind, nil).want(t, http.StatusOK))
		if list.Folder != "Marshal" || len(list.Files) != 1 || list.Files[0].Name != want {
			t.Errorf("kind %s: %+v, want just %s in the folder Marshal", kind, list, want)
		}
	}
	all := decode[protocol.GoogleFiles](t, st.do(http.MethodGet, "/v1/google/files", nil).want(t, http.StatusOK))
	if len(all.Files) != 4 {
		t.Errorf("with no kind: %d files, want all four and no folder", len(all.Files))
	}
	st.do(http.MethodGet, "/v1/google/files?kind=movie", nil).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	reads := map[string]struct {
		kind     protocol.GoogleFileKind
		title    string
		markdown string
	}{
		"https://docs.google.com/document/d/DOC1/edit":       {protocol.GoogleFileKindDoc, "Notes", "## Hello"},
		"https://docs.google.com/spreadsheets/d/SHEET1/edit": {protocol.GoogleFileKindSheet, "Budget", "| a | b |\n| --- | --- |\n| 1 | 2 |"},
		"https://docs.google.com/presentation/d/DECK1/edit":  {protocol.GoogleFileKindSlides, "Deck", "## 1. Hi"},
	}
	for link, want := range reads {
		got := decode[protocol.GoogleLinkContent](t, st.do(http.MethodPost, "/v1/google/read", protocol.ReadGoogleLinkRequest{URL: link}).
			want(t, http.StatusOK))
		if got.Kind != want.kind || got.Title != want.title || got.Markdown != want.markdown || got.URL != link || got.Truncated {
			t.Errorf("read %s = %+v", link, got)
		}
	}
	notGoogle := st.do(http.MethodPost, "/v1/google/read", protocol.ReadGoogleLinkRequest{URL: "https://example.com/x"}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if notGoogle.Message != "Paste the address of a Google Doc, Sheet or Slides presentation." {
		t.Errorf("message = %q", notGoogle.Message)
	}
	missing := st.do(http.MethodPost, "/v1/google/read", protocol.ReadGoogleLinkRequest{URL: "https://docs.google.com/document/d/NOPE/edit"}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if missing.Message != "Marshal could not open that file. Check the link, and that the Google account you connected can open it." {
		t.Errorf("message = %q", missing.Message)
	}
}

func TestGoogleRefusalsNameTheServiceThatIsNotConnected(t *testing.T) {
	st, _, _ := newGoogleFilesStack(t)
	notConnected := func(name string) string {
		return name + " is not connected yet. Connect it in Settings, under Integrations."
	}
	posts := []struct {
		path string
		body any
		name string
	}{
		{"/v1/google/docs", protocol.CreateGoogleDocRequest{Title: "T", Text: "x"}, "Google Docs"},
		{"/v1/google/sheets", protocol.CreateGoogleSheetRequest{Title: "T", Rows: [][]string{{"a"}}}, "Google Sheets"},
		{"/v1/google/slides", protocol.CreateGoogleSlidesRequest{Title: "T", Slides: []protocol.GoogleSlide{{Title: "a"}}}, "Google Slides"},
		{"/v1/google/drive/files", protocol.UploadGoogleFileRequest{Name: "a", Content: "x"}, "Google Drive"},
		{"/v1/google/read", protocol.ReadGoogleLinkRequest{URL: "https://docs.google.com/spreadsheets/d/S1/edit"}, "Google Sheets"},
		{"/v1/google/read", protocol.ReadGoogleLinkRequest{URL: "https://docs.google.com/presentation/d/S1/edit"}, "Google Slides"},
		{"/v1/google/read", protocol.ReadGoogleLinkRequest{URL: "https://docs.google.com/document/d/S1/edit"}, "Google Docs"},
	}
	for _, tc := range posts {
		got := st.do(http.MethodPost, tc.path, tc.body).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
		if got.Message != notConnected(tc.name) {
			t.Errorf("%s: %q, want %q", tc.path, got.Message, notConnected(tc.name))
		}
	}
	lists := map[string]string{"": "Google Drive", "file": "Google Drive", "doc": "Google Docs", "sheet": "Google Sheets", "slides": "Google Slides"}
	for kind, name := range lists {
		got := st.do(http.MethodGet, "/v1/google/files?kind="+kind, nil).apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
		if got.Message != notConnected(name) {
			t.Errorf("list %q: %q", kind, got.Message)
		}
	}
}

func TestGoogleRefusalsAskToReconnectByNameWhenGoogleNoLongerAcceptsTheAccess(t *testing.T) {
	st, fake, files := newGoogleFilesStack(t)
	connectGoogleFile(t, st, fake, "gdocs")
	files.Unauthorized.Store(true)
	got := st.do(http.MethodPost, "/v1/google/docs", protocol.CreateGoogleDocRequest{Title: "T", Text: "x"}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Message != "Google no longer accepts Marshal's access to Google Docs. Reconnect it in Settings." {
		t.Errorf("message = %q", got.Message)
	}
}

func TestGoogleSaysWhichAPIIsTurnedOffWhenAFileCannotBeMade(t *testing.T) {
	st, fake, files := newGoogleFilesStack(t)
	connectGoogleFile(t, st, fake, "gslides")
	files.TurnOff("slides")
	got := st.do(http.MethodPost, "/v1/google/slides", protocol.CreateGoogleSlidesRequest{Title: "T", Slides: []protocol.GoogleSlide{{Title: "a"}}}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Message != "Turn on the Google Slides API in the Google Cloud project Marshal signs in with." {
		t.Errorf("message = %q", got.Message)
	}
}

func TestGoogleFileBodiesMayBeBiggerThanOtherRequestsUpToTwoMegabytes(t *testing.T) {
	st, fake, _ := newGoogleFilesStack(t)
	connectGoogleFile(t, st, fake, "gdocs")
	// More than the 1 MiB an ordinary request may carry, and inside what a document may hold.
	fits := strings.Repeat("a", 1500*1024)
	st.do(http.MethodPost, "/v1/google/docs", protocol.CreateGoogleDocRequest{Title: "Big", Text: fits}).want(t, http.StatusCreated)
	tooBig := strings.Repeat("a", 2<<20+1)
	got := st.do(http.MethodPost, "/v1/google/docs", protocol.CreateGoogleDocRequest{Title: "Bigger", Text: tooBig}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if got.Message != "That document is too big. Marshal saves up to 2 MB at a time." {
		t.Errorf("message = %q", got.Message)
	}
}

func TestGoogleRoutesRaiseTheBodyLimitAndNoOtherRouteDoes(t *testing.T) {
	st, fake, _ := newGoogleFilesStack(t, withLimits(api.Limits{MaxBodyBytes: 4096}))
	connectGoogleFile(t, st, fake, "gdocs")
	st.do(http.MethodPost, "/v1/google/docs", protocol.CreateGoogleDocRequest{Title: "Big", Text: strings.Repeat("a", 100*1024)}).
		want(t, http.StatusCreated)
	st.do(http.MethodPut, "/v1/integrations/gdrive", protocol.SaveGoogleDriveRequest{Folder: strings.Repeat("a", 5000)}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}
