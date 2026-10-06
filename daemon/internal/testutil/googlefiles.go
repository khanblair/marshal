package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// GoogleRequest is one call the fake Google files server received.
type GoogleRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   []byte
}

// GoogleFile is one file the fake holds: one a test made through Marshal, or one the test put there.
type GoogleFile struct {
	ID        string
	Name      string
	MimeType  string
	Parents   []string
	Media     string
	MediaType string
}

// GoogleFiles is a fake of the Drive, Docs, Sheets and Slides APIs on one address
// (integrations.Options.GoogleFilesBaseURL). It makes and lists files, answers the reads a test gives
// it, and can say an API is turned off or a token refused.
type GoogleFiles struct {
	// URL is where the fake listens.
	URL string

	// Unauthorized makes every call answer 401, as Google does for a token that was revoked.
	Unauthorized atomic.Bool

	mu       sync.Mutex
	requests []GoogleRequest
	files    []GoogleFile
	nextID   int
	folderID string
	deleted  map[string]bool
	off      map[string]bool
	docs     map[string]string
	sheets   map[string][2]string
	decks    map[string]string
}

// NewGoogleFiles starts the fake and stops it when the test ends.
func NewGoogleFiles(t *testing.T) *GoogleFiles {
	t.Helper()
	g := &GoogleFiles{off: map[string]bool{}, deleted: map[string]bool{}, docs: map[string]string{}, sheets: map[string][2]string{}, decks: map[string]string{}}
	server := httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(server.Close)
	g.URL = server.URL
	return g
}

// TurnOff makes one API ("drive", "docs", "sheets" or "slides") answer that it is not turned on.
func (g *GoogleFiles) TurnOff(api string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.off[api] = true
}

// SetFolder says the folder Marshal looks for is already there, with this id.
func (g *GoogleFiles) SetFolder(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.folderID = id
}

// DeleteFolder says the person deleted Marshal's folder: a file made in it is not found, and a search
// for the folder finds none.
func (g *GoogleFiles) DeleteFolder() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.deleted[g.folderID] = true
	g.folderID = ""
}

// SetDoc gives the fake a document: the JSON documents.get answers with.
func (g *GoogleFiles) SetDoc(id, documentJSON string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.docs[id] = documentJSON
}

// SetSheet gives the fake a spreadsheet: the JSON of its tabs and the JSON of its first tab's values.
func (g *GoogleFiles) SetSheet(id, metaJSON, valuesJSON string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sheets[id] = [2]string{metaJSON, valuesJSON}
}

// SetDeck gives the fake a presentation: the JSON presentations.get answers with.
func (g *GoogleFiles) SetDeck(id, presentationJSON string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.decks[id] = presentationJSON
}

// Requests is every call received, in order.
func (g *GoogleFiles) Requests() []GoogleRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]GoogleRequest(nil), g.requests...)
}

// Made is every file the fake holds, oldest first.
func (g *GoogleFiles) Made() []GoogleFile {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]GoogleFile(nil), g.files...)
}

const (
	notFoundJSON   = `{"error":{"code":404,"message":"Requested entity was not found.","status":"NOT_FOUND"}}`
	unauthedJSON   = `{"error":{"code":401,"message":"Invalid Credentials","status":"UNAUTHENTICATED"}}`
	driveOffJSON   = `{"error":{"code":403,"message":"Google Drive API has not been used in project 1 before or it is disabled.","errors":[{"reason":"accessNotConfigured"}],"status":"PERMISSION_DENIED"}}`
	otherOffJSON   = `{"error":{"code":403,"message":"The API has not been used in project 1 before or it is disabled.","status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"SERVICE_DISABLED"}]}}`
	parentGoneJSON = `{"error":{"code":404,"message":"File not found: the folder.","errors":[{"reason":"notFound"}]}}`
	folderMime     = "application/vnd.google-apps.folder"
	probeFileName  = "marshal-connection-check"
)

// apiOf says which API a path belongs to.
func apiOf(path string) string {
	switch {
	case strings.HasPrefix(path, "/v1/documents/"):
		return "docs"
	case strings.HasPrefix(path, "/v4/spreadsheets/"):
		return "sheets"
	case strings.HasPrefix(path, "/v1/presentations/"):
		return "slides"
	}
	return "drive"
}

func (g *GoogleFiles) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	g.mu.Lock()
	g.requests = append(g.requests, GoogleRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Body: body})
	api := apiOf(r.URL.Path)
	off := g.off[api]
	g.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case g.Unauthorized.Load():
		answer(w, http.StatusUnauthorized, unauthedJSON)
	case off && api == "drive":
		answer(w, http.StatusForbidden, driveOffJSON)
	case off:
		answer(w, http.StatusForbidden, otherOffJSON)
	default:
		g.route(w, r, body)
	}
}

func answer(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func (g *GoogleFiles) route(w http.ResponseWriter, r *http.Request, body []byte) {
	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && path == "/drive/v3/files":
		g.list(w, r)
	case r.Method == http.MethodPost && path == "/drive/v3/files":
		g.createFromJSON(w, r, body)
	case r.Method == http.MethodPost && path == "/upload/drive/v3/files":
		g.upload(w, r, body)
	case r.Method == http.MethodPost && strings.HasSuffix(path, ":batchUpdate"):
		answer(w, http.StatusOK, `{"replies":[]}`)
	case r.Method == http.MethodGet:
		g.read(w, r)
	default:
		answer(w, http.StatusNotFound, notFoundJSON)
	}
}

// fileJSON writes a file the way Drive does: the link only when the call's fields ask for it.
func fileJSON(file GoogleFile, fields string) string {
	out := map[string]any{"id": file.ID, "name": file.Name, "mimeType": file.MimeType, "modifiedTime": "2026-10-05T10:00:00.000Z"}
	if strings.Contains(fields, "webViewLink") {
		out["webViewLink"] = "https://drive.google.com/open?id=" + file.ID
	}
	raw, _ := json.Marshal(out)
	return string(raw)
}

func (g *GoogleFiles) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	g.mu.Lock()
	defer g.mu.Unlock()
	if strings.Contains(q, "mimeType='"+folderMime+"'") {
		if g.folderID == "" {
			answer(w, http.StatusOK, `{"files":[]}`)
			return
		}
		answer(w, http.StatusOK, `{"files":[{"id":"`+g.folderID+`"}]}`)
		return
	}
	var items []string
	for i := len(g.files) - 1; i >= 0 && len(items) < 20; i-- {
		file := g.files[i]
		if file.MimeType == folderMime || !matches(q, file.MimeType) {
			continue
		}
		items = append(items, fileJSON(file, "webViewLink"))
	}
	answer(w, http.StatusOK, `{"files":[`+strings.Join(items, ",")+`]}`)
}

// matches reads the part of a list's search that picks a kind: one type it equals, or the types it
// leaves out.
func matches(q, mimeType string) bool {
	if strings.Contains(q, "mimeType='"+mimeType+"'") {
		return true
	}
	if strings.Contains(q, "mimeType='") && !strings.Contains(q, "mimeType!='") {
		return false
	}
	return !strings.Contains(q, "mimeType!='"+mimeType+"'")
}

func (g *GoogleFiles) add(file GoogleFile) GoogleFile {
	g.nextID++
	file.ID = fmt.Sprintf("file-%d", g.nextID)
	if file.MimeType == folderMime {
		file.ID = fmt.Sprintf("folder-%d", g.nextID)
		g.folderID = file.ID
	}
	g.files = append(g.files, file)
	return file
}

// parentGone says a new file names a folder that was deleted.
func (g *GoogleFiles) parentGone(parents []string) bool {
	for _, parent := range parents {
		if g.deleted[parent] {
			return true
		}
	}
	return false
}

type metaJSON struct {
	Name     string   `json:"name"`
	MimeType string   `json:"mimeType"`
	Parents  []string `json:"parents"`
}

func (g *GoogleFiles) createFromJSON(w http.ResponseWriter, r *http.Request, body []byte) {
	var meta metaJSON
	if err := json.Unmarshal(body, &meta); err != nil {
		answer(w, http.StatusBadRequest, `{"error":{"code":400,"message":"bad json"}}`)
		return
	}
	g.mu.Lock()
	if g.parentGone(meta.Parents) {
		g.mu.Unlock()
		answer(w, http.StatusNotFound, parentGoneJSON)
		return
	}
	file := g.add(GoogleFile{Name: meta.Name, MimeType: meta.MimeType, Parents: meta.Parents})
	g.mu.Unlock()
	answer(w, http.StatusOK, fileJSON(file, r.URL.Query().Get("fields")))
}

func (g *GoogleFiles) upload(w http.ResponseWriter, r *http.Request, body []byte) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/related" {
		answer(w, http.StatusBadRequest, `{"error":{"code":400,"message":"not multipart/related"}}`)
		return
	}
	reader := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
	var meta metaJSON
	var file GoogleFile
	for index := 0; ; index++ {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		raw, _ := io.ReadAll(part)
		if index == 0 {
			_ = json.Unmarshal(raw, &meta)
			continue
		}
		file.Media, file.MediaType = string(raw), part.Header.Get("Content-Type")
	}
	file.Name, file.MimeType, file.Parents = meta.Name, meta.MimeType, meta.Parents
	if file.MimeType == "" {
		file.MimeType = file.MediaType
	}
	g.mu.Lock()
	if g.parentGone(file.Parents) {
		g.mu.Unlock()
		answer(w, http.StatusNotFound, parentGoneJSON)
		return
	}
	file = g.add(file)
	g.mu.Unlock()
	answer(w, http.StatusOK, fileJSON(file, r.URL.Query().Get("fields")))
}

// read answers the Docs, Sheets and Slides reads, and the made-up file a connection test asks for.
func (g *GoogleFiles) read(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	g.mu.Lock()
	defer g.mu.Unlock()
	switch api := apiOf(path); api {
	case "docs":
		if doc, ok := g.docs[strings.TrimPrefix(path, "/v1/documents/")]; ok {
			answer(w, http.StatusOK, doc)
			return
		}
	case "sheets":
		rest := strings.TrimPrefix(path, "/v4/spreadsheets/")
		id, values, hasValues := strings.Cut(rest, "/values/")
		if sheet, ok := g.sheets[id]; ok {
			if hasValues && values != "" {
				answer(w, http.StatusOK, sheet[1])
				return
			}
			answer(w, http.StatusOK, sheet[0])
			return
		}
	case "slides":
		id := strings.TrimPrefix(path, "/v1/presentations/")
		if deck, ok := g.decks[id]; ok {
			answer(w, http.StatusOK, deck)
			return
		}
		if r.URL.Query().Get("fields") == "slides.objectId" && id != probeFileName {
			answer(w, http.StatusOK, `{"slides":[{"objectId":"p_blank"}]}`)
			return
		}
	}
	answer(w, http.StatusNotFound, notFoundJSON)
}
