package googlefiles

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// seen is one request the fake Google received.
type seen struct {
	method, path, rawQuery string
	query                  map[string][]string
	header                 http.Header
	body                   []byte
}

// recorder is a Google that remembers what it was asked and answers with a function the test gives.
type recorder struct {
	mu       sync.Mutex
	requests []seen
	answer   func(r *http.Request, body []byte) (status int, json string)
}

func (g *recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	g.mu.Lock()
	g.requests = append(g.requests, seen{
		method: r.Method, path: r.URL.Path, rawQuery: r.URL.RawQuery, query: r.URL.Query(), header: r.Header.Clone(), body: body,
	})
	g.mu.Unlock()
	status, answer := g.answer(r, body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(answer))
}

func (g *recorder) all() []seen {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]seen(nil), g.requests...)
}

// newClient starts a fake Google that answers every request with answer, and a Client pointed at it.
func newClient(t *testing.T, answer func(r *http.Request, body []byte) (int, string)) (*Client, *recorder) {
	t.Helper()
	g := &recorder{answer: answer}
	server := httptest.NewServer(g)
	t.Cleanup(server.Close)
	c := New(server.Client(), server.URL+"/")
	c.retryDelay = time.Millisecond
	return c, g
}

func always(status int, body string) func(*http.Request, []byte) (int, string) {
	return func(*http.Request, []byte) (int, string) { return status, body }
}

func TestTheRealHostsAreUsedWhenNoBaseIsGiven(t *testing.T) {
	c := New(http.DefaultClient, "")
	tests := map[string]string{
		c.endpoint(APIDrive, "/drive/v3/files", nil):                    "https://www.googleapis.com/drive/v3/files",
		c.endpoint(APIDrive, "/upload/drive/v3/files", nil):             "https://www.googleapis.com/upload/drive/v3/files",
		c.endpoint(APIDocs, "/v1/documents/abc", nil):                   "https://docs.googleapis.com/v1/documents/abc",
		c.endpoint(APISheets, "/v4/spreadsheets/abc", nil):              "https://sheets.googleapis.com/v4/spreadsheets/abc",
		c.endpoint(APISlides, "/v1/presentations/abc:batchUpdate", nil): "https://slides.googleapis.com/v1/presentations/abc:batchUpdate",
	}
	for got, want := range tests {
		if got != want {
			t.Errorf("address = %q, want %q", got, want)
		}
	}
	if got := New(http.DefaultClient, "http://127.0.0.1:9/").endpoint(APIDocs, "/v1/documents/x", nil); got != "http://127.0.0.1:9/v1/documents/x" {
		t.Errorf("with a base, address = %q", got)
	}
}

func TestAFolderNameIsEscapedBackslashFirst(t *testing.T) {
	if got := EscapeQuery(`Mom's \ files`); got != `Mom\'s \\ files` {
		t.Errorf("escaped = %q", got)
	}
	c, g := newClient(t, always(200, `{"files":[{"id":"folder-1"}]}`))
	id, err := c.FindFolder(context.Background(), `Mom's \ files`)
	if err != nil || id != "folder-1" {
		t.Fatalf("FindFolder = %q, %v", id, err)
	}
	req := g.all()[0]
	want := `mimeType='application/vnd.google-apps.folder' and name='Mom\'s \\ files' and trashed=false`
	if req.path != "/drive/v3/files" || req.query["q"][0] != want {
		t.Errorf("asked %s q=%q, want q=%q", req.path, req.query["q"], want)
	}
	if req.query["orderBy"][0] != "createdTime" || req.query["pageSize"][0] != "1" {
		t.Errorf("query = %v, want the oldest folder first", req.query)
	}
}

func TestNoFolderIsAnEmptyIdAndMakingOneSendsItsNameAndType(t *testing.T) {
	c, _ := newClient(t, always(200, `{"files":[]}`))
	if id, err := c.FindFolder(context.Background(), "Marshal"); err != nil || id != "" {
		t.Errorf("no folder = %q, %v, want an empty id", id, err)
	}
	c, g := newClient(t, always(200, `{"id":"new-folder"}`))
	id, err := c.MakeFolder(context.Background(), "Marshal")
	if err != nil || id != "new-folder" {
		t.Fatalf("MakeFolder = %q, %v", id, err)
	}
	req := g.all()[0]
	var meta map[string]any
	if err := json.Unmarshal(req.body, &meta); err != nil {
		t.Fatal(err)
	}
	if req.method != http.MethodPost || req.path != "/drive/v3/files" || meta["name"] != "Marshal" || meta["mimeType"] != MimeFolder {
		t.Errorf("made it with %s %s %v", req.method, req.path, meta)
	}
	if _, has := meta["parents"]; has {
		t.Errorf("the folder was filed under %v, want the top of Drive", meta["parents"])
	}
	if !strings.HasPrefix(req.header.Get("Content-Type"), "application/json") {
		t.Errorf("content type = %q", req.header.Get("Content-Type"))
	}
}

// parts reads a multipart/related upload into the type and body of each part.
func parts(t *testing.T, req seen) (types []string, bodies []string) {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(req.header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/related" {
		t.Fatalf("content type = %q (%v), want multipart/related", req.header.Get("Content-Type"), err)
	}
	reader := multipart.NewReader(strings.NewReader(string(req.body)), params["boundary"])
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return types, bodies
		}
		if err != nil {
			t.Fatalf("read the upload: %v", err)
		}
		raw, _ := io.ReadAll(part)
		types = append(types, part.Header.Get("Content-Type"))
		bodies = append(bodies, string(raw))
	}
}

// driveCreate answers an upload the way Drive does: webViewLink only when the fields ask for it.
func driveCreate(r *http.Request, _ []byte) (int, string) {
	link := ""
	if strings.Contains(r.URL.Query().Get("fields"), "webViewLink") {
		link = `,"webViewLink":"https://docs.google.com/document/d/made-1/edit?usp=drivesdk"`
	}
	return 200, `{"id":"made-1","name":"Notes","mimeType":"application/vnd.google-apps.document"` + link + `}`
}

func TestADocIsUploadedAsHTMLInOneMultipartCallAndKeepsItsLink(t *testing.T) {
	c, g := newClient(t, driveCreate)
	file, err := c.CreateDoc(context.Background(), "folder-1", "Notes", "<h1>Hi</h1>", true)
	if err != nil {
		t.Fatal(err)
	}
	if file.ID != "made-1" || file.URL != "https://docs.google.com/document/d/made-1/edit?usp=drivesdk" {
		t.Errorf("file = %+v, want the link Drive gave", file)
	}
	req := g.all()[0]
	if req.path != "/upload/drive/v3/files" || req.query["uploadType"][0] != "multipart" {
		t.Errorf("asked %s %v", req.path, req.query)
	}
	if fields := req.query["fields"][0]; fields != "id,name,mimeType,webViewLink,modifiedTime" {
		t.Errorf("fields = %q", fields)
	}
	types, bodies := parts(t, req)
	if len(types) != 2 || types[0] != "application/json; charset=UTF-8" || types[1] != "text/html; charset=utf-8" {
		t.Fatalf("part types = %q", types)
	}
	var meta struct {
		Name     string   `json:"name"`
		MimeType string   `json:"mimeType"`
		Parents  []string `json:"parents"`
	}
	if err := json.Unmarshal([]byte(bodies[0]), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Name != "Notes" || meta.MimeType != MimeDoc || len(meta.Parents) != 1 || meta.Parents[0] != "folder-1" {
		t.Errorf("details = %+v", meta)
	}
	if bodies[1] != "<h1>Hi</h1>" {
		t.Errorf("content = %q", bodies[1])
	}
}

func TestATextDocIsSentAsPlainText(t *testing.T) {
	c, g := newClient(t, driveCreate)
	if _, err := c.CreateDoc(context.Background(), "", "Notes", "just words", false); err != nil {
		t.Fatal(err)
	}
	types, bodies := parts(t, g.all()[0])
	if types[1] != "text/plain; charset=utf-8" || bodies[1] != "just words" {
		t.Errorf("content part = %q %q", types[1], bodies[1])
	}
	if strings.Contains(bodies[0], "parents") {
		t.Errorf("details = %s, want no parents when there is no folder", bodies[0])
	}
}

func TestAFileWithNoLinkGetsTheAddressGoogleOpensItAt(t *testing.T) {
	tests := []struct {
		mime, want string
	}{
		{MimeDoc, "https://docs.google.com/document/d/x/edit"},
		{MimeSheet, "https://docs.google.com/spreadsheets/d/x/edit"},
		{MimeSlides, "https://docs.google.com/presentation/d/x/edit"},
		{"text/markdown", "https://drive.google.com/file/d/x/view"},
	}
	for _, tc := range tests {
		got := fileOf(driveFile{ID: "x", MimeType: tc.mime, ModifiedTime: "2026-10-05T10:11:12.123Z"})
		if got.URL != tc.want {
			t.Errorf("%s: URL = %q, want %q", tc.mime, got.URL, tc.want)
		}
		if got.ModifiedAt.IsZero() {
			t.Errorf("%s: the modified time was not read", tc.mime)
		}
	}
}

func TestACellThatCouldRunAsAFormulaStaysText(t *testing.T) {
	tests := map[string]string{
		"=SUM(A1)":   "'=SUM(A1)",
		"+1+2":       "'+1+2",
		"@cmd":       "'@cmd",
		"\tdata":     "'\tdata",
		"\rdata":     "'\rdata",
		"-cmd|x":     "'-cmd|x",
		"-5":         "-5",
		"-3.25":      "-3.25",
		"-.5":        "-.5",
		"-1e5":       "-1e5",
		"-$5":        "'-$5",
		"-inf":       "'-inf",
		"- 5":        "'- 5",
		"plain":      "plain",
		"a=b":        "a=b",
		"":           "",
		"-":          "'-",
		"5 - 3":      "5 - 3",
		"=cmd|' /C'": "'=cmd|' /C'",
	}
	for cell, want := range tests {
		if got := neutralize(cell); got != want {
			t.Errorf("neutralize(%q) = %q, want %q", cell, got, want)
		}
	}
}

func TestASheetIsUploadedAsCSVWithQuotingAndTheFormulaGuard(t *testing.T) {
	c, g := newClient(t, func(*http.Request, []byte) (int, string) {
		return 200, `{"id":"sheet-1","name":"Board","mimeType":"application/vnd.google-apps.spreadsheet"}`
	})
	rows := [][]string{{"Title", "Note"}, {"Fix, now", "=HYPERLINK(\"x\")"}, {"Two\nlines", `say "hi"`}}
	file, err := c.CreateSheet(context.Background(), "folder-1", "Board", rows)
	if err != nil {
		t.Fatal(err)
	}
	if file.URL != "https://docs.google.com/spreadsheets/d/sheet-1/edit" {
		t.Errorf("URL = %q", file.URL)
	}
	types, bodies := parts(t, g.all()[0])
	if types[1] != "text/csv; charset=utf-8" {
		t.Errorf("content type = %q", types[1])
	}
	want := "Title,Note\n" + `"Fix, now","'=HYPERLINK(""x"")"` + "\n" + "\"Two\nlines\"," + `"say ""hi"""` + "\n"
	if bodies[1] != want {
		t.Errorf("csv = %q, want %q", bodies[1], want)
	}
	if !strings.Contains(bodies[0], MimeSheet) {
		t.Errorf("details = %s, want the sheet type", bodies[0])
	}
}

func TestAPlainFileIsSavedWithItsOwnTypeAndNoConversion(t *testing.T) {
	c, g := newClient(t, always(200, `{"id":"f-1","name":"a.md","mimeType":"text/markdown"}`))
	if _, err := c.CreateFile(context.Background(), "folder-1", "a.md", "text/markdown", "# hi"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateFile(context.Background(), "folder-1", "b.txt", "", "words"); err != nil {
		t.Fatal(err)
	}
	reqs := g.all()
	types, bodies := parts(t, reqs[0])
	if types[1] != "text/markdown" || bodies[1] != "# hi" || strings.Contains(bodies[0], "mimeType") {
		t.Errorf("first upload = %q %q details %s", types[1], bodies[1], bodies[0])
	}
	if types, _ := parts(t, reqs[1]); types[1] != "text/plain; charset=utf-8" {
		t.Errorf("a file with no type was sent as %q, want plain text", types[1])
	}
}

func TestAnUploadTypeThatCouldInjectAHeaderOrConvertTheFileIsRefused(t *testing.T) {
	c, g := newClient(t, always(200, `{"id":"x"}`))
	for _, bad := range []string{
		"text/plain\r\nX-Evil: 1", "text/plain\nX-Evil: 1", "not a type", "text/plain; charset=\r\n", MimeDoc, MimeSheet,
		"application/vnd.google-apps.script",
	} {
		if _, err := c.CreateFile(context.Background(), "", "a", bad, "x"); err == nil {
			t.Errorf("the type %q was accepted", bad)
		}
	}
	if len(g.all()) != 0 {
		t.Errorf("%d requests reached Google, want none for a refused type", len(g.all()))
	}
	if got, err := CleanMediaType("Text/Markdown; Charset=UTF-8"); err != nil || got != "text/markdown; charset=UTF-8" {
		t.Errorf("CleanMediaType = %q, %v", got, err)
	}
}

func TestAPresentationIsMadeBlankThenFilledInOneBatchAndTheBlankSlideRemoved(t *testing.T) {
	c, g := newClient(t, func(r *http.Request, _ []byte) (int, string) {
		switch {
		case r.URL.Path == "/drive/v3/files":
			return 200, `{"id":"deck-1","name":"Plan","mimeType":"application/vnd.google-apps.presentation"}`
		case r.Method == http.MethodGet:
			return 200, `{"slides":[{"objectId":"p"}]}`
		}
		return 200, `{"presentationId":"deck-1","replies":[]}`
	})
	slides := []Slide{{Title: "First", Bullets: []string{"a", "b"}}, {Title: "Second"}, {Bullets: []string{"only body"}}}
	file, err := c.CreateSlides(context.Background(), "folder-1", "Plan", slides)
	if err != nil {
		t.Fatal(err)
	}
	if file.ID != "deck-1" || file.URL != "https://docs.google.com/presentation/d/deck-1/edit" {
		t.Errorf("file = %+v", file)
	}
	reqs := g.all()
	if len(reqs) != 3 {
		t.Fatalf("%d calls, want create, read the blank slide, one batch", len(reqs))
	}
	var meta map[string]any
	if err := json.Unmarshal(reqs[0].body, &meta); err != nil || meta["mimeType"] != MimeSlides ||
		meta["parents"].([]any)[0] != "folder-1" {
		t.Errorf("create sent %s", reqs[0].body)
	}
	if reqs[1].path != "/v1/presentations/deck-1" || reqs[1].query["fields"][0] != "slides.objectId" {
		t.Errorf("blank slide read = %s %v", reqs[1].path, reqs[1].query)
	}
	if reqs[2].method != http.MethodPost || reqs[2].path != "/v1/presentations/deck-1:batchUpdate" {
		t.Fatalf("batch = %s %s", reqs[2].method, reqs[2].path)
	}
	var batch struct {
		Requests []map[string]json.RawMessage `json:"requests"`
	}
	if err := json.Unmarshal(reqs[2].body, &batch); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, request := range batch.Requests {
		for kind := range request {
			kinds = append(kinds, kind)
		}
	}
	// Three slides, with text for the first (title and body), the second (title), the third (body).
	want := "createSlide insertText insertText createSlide insertText createSlide insertText deleteObject"
	if strings.Join(kinds, " ") != want {
		t.Errorf("requests = %s, want %s", strings.Join(kinds, " "), want)
	}
	first := string(batch.Requests[0]["createSlide"])
	for _, piece := range []string{`"objectId":"m_slide_1"`, `"predefinedLayout":"TITLE_AND_BODY"`, `"objectId":"m_title_1"`, `"objectId":"m_body_1"`,
		`"type":"TITLE"`, `"type":"BODY"`} {
		if !strings.Contains(first, piece) {
			t.Errorf("createSlide %s lacks %s", first, piece)
		}
	}
	if body := string(batch.Requests[2]["insertText"]); !strings.Contains(body, `"objectId":"m_body_1"`) || !strings.Contains(body, `"text":"a\nb"`) {
		t.Errorf("body text = %s", body)
	}
	if last := string(batch.Requests[len(batch.Requests)-1]["deleteObject"]); last != `{"objectId":"p"}` {
		t.Errorf("last request = %s, want the blank slide removed", last)
	}
}

func TestAFailedFillNamesThePresentationThatWasMade(t *testing.T) {
	c, _ := newClient(t, func(r *http.Request, _ []byte) (int, string) {
		if r.URL.Path == "/drive/v3/files" {
			return 200, `{"id":"deck-1"}`
		}
		if r.Method == http.MethodGet {
			return 200, `{"slides":[]}`
		}
		return 400, `{"error":{"code":400,"message":"bad request","status":"INVALID_ARGUMENT"}}`
	})
	_, err := c.CreateSlides(context.Background(), "", "Plan", []Slide{{Title: "x"}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 400 || !strings.Contains(err.Error(), "deck-1") {
		t.Errorf("error = %v, want the 400 and the presentation's id", err)
	}
}

func TestEachKindOfListAsksDriveForItsOwnTypeNewestFirst(t *testing.T) {
	c, g := newClient(t, always(200, `{"files":[
		{"id":"a","name":"A","mimeType":"application/vnd.google-apps.document","webViewLink":"https://x/a","modifiedTime":"2026-10-05T10:00:00.000Z"},
		{"id":"b","name":"B","mimeType":"text/markdown"}]}`))
	files, err := c.List(context.Background(), KindAny)
	if err != nil || len(files) != 2 || files[0].Kind() != KindDoc || files[1].Kind() != KindFile || files[0].URL != "https://x/a" {
		t.Fatalf("List = %+v, %v", files, err)
	}
	for _, kind := range []Kind{KindDoc, KindSheet, KindSlides, KindFile} {
		if _, err := c.List(context.Background(), kind); err != nil {
			t.Fatal(err)
		}
	}
	wants := []string{
		"trashed=false and mimeType!='application/vnd.google-apps.folder'",
		"trashed=false and mimeType='application/vnd.google-apps.document'",
		"trashed=false and mimeType='application/vnd.google-apps.spreadsheet'",
		"trashed=false and mimeType='application/vnd.google-apps.presentation'",
		"trashed=false and mimeType!='application/vnd.google-apps.folder' and mimeType!='application/vnd.google-apps.document' and " +
			"mimeType!='application/vnd.google-apps.spreadsheet' and mimeType!='application/vnd.google-apps.presentation'",
	}
	for i, req := range g.all() {
		q := req.query
		if q["q"][0] != wants[i] || q["orderBy"][0] != "modifiedTime desc" || q["pageSize"][0] != "20" ||
			q["fields"][0] != "files(id,name,mimeType,webViewLink,modifiedTime)" {
			t.Errorf("list %d asked %v, want q=%q", i, q, wants[i])
		}
	}
	empty, _ := newClient(t, always(200, `{}`))
	if files, err := empty.List(context.Background(), KindDoc); err != nil || files == nil || len(files) != 0 {
		t.Errorf("an empty list = %#v, %v, want an empty slice and not nil", files, err)
	}
}

func TestAnErrorReadsDrivesReasonAndTheOtherThreeAPIsReasonAlike(t *testing.T) {
	drive := answerError(APIDrive, 403, []byte(`{"error":{"code":403,"message":"Google Drive API has not been used in project 1 before or it is disabled.",
		"errors":[{"message":"x","domain":"usageLimits","reason":"accessNotConfigured"}],"status":"PERMISSION_DENIED"}}`))
	docs := answerError(APIDrive, 403, []byte(`{"error":{"code":403,"message":"Google Docs API has not been used in project 1 before or it is disabled.",
		"status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"SERVICE_DISABLED","domain":"googleapis.com"}]}}`))
	both := answerError(APIDrive, 403, []byte(`{"error":{"code":403,"message":"m","errors":[{"reason":"forbidden"}],
		"details":[{"reason":"SERVICE_DISABLED"}]}}`))
	worded := answerError(APIDrive, 403, []byte(`{"error":{"code":403,"message":"The Slides API is disabled in project 2.","status":"PERMISSION_DENIED"}}`))
	for name, got := range map[string]*APIError{"drive": drive, "docs": docs, "both": both, "worded": worded} {
		if !got.NotEnabled() {
			t.Errorf("%s: %+v is not read as an API that is off", name, got)
		}
	}
	if drive.Reason != "accessNotConfigured" || docs.Reason != "SERVICE_DISABLED" || both.Reason != "SERVICE_DISABLED" {
		t.Errorf("reasons = %q %q %q", drive.Reason, docs.Reason, both.Reason)
	}
	denied := answerError(APIDrive, 403, []byte(`{"error":{"code":403,"message":"The caller does not have permission","status":"PERMISSION_DENIED"}}`))
	if denied.NotEnabled() || denied.Reason != "PERMISSION_DENIED" || denied.Temporary() {
		t.Errorf("a plain permission refusal = %+v", denied)
	}
	html := answerError(APIDocs, 502, []byte("<html>"+strings.Repeat("x", 500)))
	if html.Status != 502 || len(html.Message) != maxErrorText || !html.Temporary() {
		t.Errorf("a body that is not Google's shape = %+v", html)
	}
	if got := drive.Error(); !strings.Contains(got, "403") || !strings.Contains(got, "accessNotConfigured") {
		t.Errorf("Error() = %q", got)
	}
}

func TestABusyGoogleIsTriedThreeTimesAndEachTryCarriesTheWholeUpload(t *testing.T) {
	calls := 0
	c, g := newClient(t, func(*http.Request, []byte) (int, string) {
		calls++
		if calls < 3 {
			return 503, `{"error":{"code":503,"message":"try later","status":"UNAVAILABLE"}}`
		}
		return 200, `{"id":"made-1","name":"Notes"}`
	})
	if _, err := c.CreateDoc(context.Background(), "folder-1", "Notes", "<p>hello</p>", true); err != nil {
		t.Fatalf("a Google that answered on the third try: %v", err)
	}
	reqs := g.all()
	if len(reqs) != 3 {
		t.Fatalf("%d tries, want 3", len(reqs))
	}
	for i, req := range reqs {
		if _, bodies := parts(t, req); len(bodies) != 2 || bodies[1] != "<p>hello</p>" {
			t.Errorf("try %d sent %q, want the whole upload each time", i+1, bodies)
		}
	}
}

func TestTheRetryGivesUpAfterThreeTriesAndSkipsAnswersThatWouldNotChange(t *testing.T) {
	c, g := newClient(t, always(500, `{"error":{"code":500,"message":"broken"}}`))
	_, err := c.List(context.Background(), KindDoc)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 500 || apiErr.API != APIDrive || len(g.all()) != 3 {
		t.Errorf("err = %v after %d calls, want the 500 from Drive after 3", err, len(g.all()))
	}
	c, g = newClient(t, always(400, `{"error":{"code":400,"message":"bad"}}`))
	if _, err := c.List(context.Background(), KindDoc); err == nil || len(g.all()) != 1 {
		t.Errorf("a 400 was tried %d times, want 1", len(g.all()))
	}
	c, g = newClient(t, always(403, `{"error":{"code":403,"message":"slow down","errors":[{"reason":"userRateLimitExceeded"}]}}`))
	if _, err := c.List(context.Background(), KindDoc); err == nil || len(g.all()) != 3 {
		t.Errorf("a quota 403 was tried %d times, want 3", len(g.all()))
	}
	c, g = newClient(t, always(429, `{}`))
	if _, err := c.List(context.Background(), KindDoc); err == nil || len(g.all()) != 3 {
		t.Errorf("a 429 was tried %d times, want 3", len(g.all()))
	}
}

func TestAWaitForAnotherTryStopsWhenTheCallIsCancelled(t *testing.T) {
	c, _ := newClient(t, always(500, `{}`))
	c.retryDelay = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.List(ctx, KindDoc); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the deadline", err)
	}
}

func TestTheProbesPassOnAnyRefusalThatIsAboutTheMadeUpFileAndFailOnTheRest(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		ok     bool
	}{
		{"not found", 404, `{"error":{"code":404,"message":"Requested entity was not found.","status":"NOT_FOUND"}}`, true},
		{"bad request", 400, `{"error":{"code":400,"message":"bad id"}}`, true},
		{"no access", 403, `{"error":{"code":403,"message":"The caller does not have permission","status":"PERMISSION_DENIED"}}`, true},
		{"token refused", 401, `{"error":{"code":401,"message":"Invalid Credentials","status":"UNAUTHENTICATED"}}`, false},
		{"api off (errors)", 403, `{"error":{"code":403,"message":"m","errors":[{"reason":"accessNotConfigured"}]}}`, false},
		{"api off (details)", 403, `{"error":{"code":403,"message":"m","details":[{"reason":"SERVICE_DISABLED"}]}}`, false},
		{"api off (words)", 403, `{"error":{"code":403,"message":"API has not been used in project 1"}}`, false},
		{"google down", 503, `{"error":{"code":503,"message":"down"}}`, false},
	}
	for _, api := range []API{APIDocs, APISheets, APISlides} {
		for _, tc := range tests {
			c, g := newClient(t, always(tc.status, tc.body))
			err := c.ProbeAPI(context.Background(), api)
			if (err == nil) != tc.ok {
				t.Errorf("%s with %s: err = %v, want ok=%v", api, tc.name, err, tc.ok)
			}
			if len(g.all()) == 0 {
				t.Fatalf("%s: nothing was asked", api)
			}
		}
	}
	paths := map[API]string{
		APIDocs: "/v1/documents/marshal-connection-check", APISheets: "/v4/spreadsheets/marshal-connection-check",
		APISlides: "/v1/presentations/marshal-connection-check",
	}
	for api, want := range paths {
		c, g := newClient(t, always(404, `{}`))
		if err := c.ProbeAPI(context.Background(), api); err != nil {
			t.Fatal(err)
		}
		if req := g.all()[0]; req.method != http.MethodGet || req.path != want {
			t.Errorf("%s probe asked %s %s, want GET %s", api, req.method, req.path, want)
		}
	}
	if err := New(http.DefaultClient, "").ProbeAPI(context.Background(), API("nope")); err == nil {
		t.Error("an API that does not exist was probed")
	}
}

func TestTheDriveProbeAsksForOneFileAndNetworkTroubleIsAnError(t *testing.T) {
	c, g := newClient(t, always(200, `{"files":[]}`))
	if err := c.ProbeDrive(context.Background()); err != nil {
		t.Fatal(err)
	}
	if req := g.all()[0]; req.path != "/drive/v3/files" || req.query["pageSize"][0] != "1" || req.method != http.MethodGet {
		t.Errorf("probe asked %s %s %v", req.method, req.path, req.query)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	down := New(http.DefaultClient, closed.URL)
	if err := down.ProbeDrive(context.Background()); err == nil {
		t.Error("a Google that cannot be reached passed the probe")
	}
	if err := down.ProbeAPI(context.Background(), APIDocs); err == nil {
		t.Error("a Google that cannot be reached passed the API probe")
	}
	if err := down.ProbeAPI(context.Background(), APIDocs); errors.As(err, new(*APIError)) || !errors.Is(err, ErrUnreachable) {
		t.Errorf("a network failure was read as %v, want ErrUnreachable and not an answer from Google", err)
	}
}
