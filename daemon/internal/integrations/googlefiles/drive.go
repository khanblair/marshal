package googlefiles

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// The Drive types of the files Marshal makes and tells apart.
const (
	MimeFolder = "application/vnd.google-apps.folder"
	MimeDoc    = "application/vnd.google-apps.document"
	MimeSheet  = "application/vnd.google-apps.spreadsheet"
	MimeSlides = "application/vnd.google-apps.presentation"

	// googleAppsPrefix starts the type of every file Google itself makes. A plain upload may not
	// name one, because Drive would convert the upload into it.
	googleAppsPrefix = "application/vnd.google-apps."
)

// Kind is what sort of file something is.
type Kind string

const (
	// KindAny asks for every kind of file but a folder.
	KindAny Kind = ""
	// KindDoc is a Google Doc.
	KindDoc Kind = "doc"
	// KindSheet is a Google Sheet.
	KindSheet Kind = "sheet"
	// KindSlides is a Google Slides presentation.
	KindSlides Kind = "slides"
	// KindFile is any other file kept in Drive.
	KindFile Kind = "file"
)

const (
	// fileFields is what Marshal asks Drive to say about a file. Drive leaves webViewLink out unless
	// it is named.
	fileFields = "id,name,mimeType,webViewLink,modifiedTime"
	// listSize is how many files one list answers.
	listSize = 20
	// textMedia and htmlMedia are the types of what Marshal sends for a document.
	textMedia = "text/plain; charset=utf-8"
	htmlMedia = "text/html; charset=utf-8"
	csvMedia  = "text/csv; charset=utf-8"
	// probeID is the id of the file that does not exist, which a connection test asks the Docs,
	// Sheets and Slides APIs for.
	probeID = "marshal-connection-check"
)

// File is one file in a person's Drive.
type File struct {
	ID       string
	Name     string
	MimeType string
	// URL opens the file in Google.
	URL string
	// ModifiedAt is zero when Google did not say.
	ModifiedAt time.Time
}

// Kind says what sort of file this is, from its Drive type.
func (f File) Kind() Kind {
	switch f.MimeType {
	case MimeDoc:
		return KindDoc
	case MimeSheet:
		return KindSheet
	case MimeSlides:
		return KindSlides
	}
	return KindFile
}

// driveFile is Drive's own shape of a file.
type driveFile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	WebViewLink  string `json:"webViewLink"`
	ModifiedTime string `json:"modifiedTime"`
}

// fileOf turns Drive's answer into a File. A file Drive gave no link for still gets the address
// Google opens it at. A modified time that does not parse is left zero.
func fileOf(item driveFile) File {
	file := File{ID: item.ID, Name: item.Name, MimeType: item.MimeType, URL: item.WebViewLink}
	if file.URL == "" {
		file.URL = ViewURL(file.Kind(), file.ID)
	}
	if when, err := time.Parse(time.RFC3339, item.ModifiedTime); err == nil {
		file.ModifiedAt = when
	}
	return file
}

// ViewURL is the address that opens a file of this kind in Google.
func ViewURL(kind Kind, id string) string {
	switch kind {
	case KindDoc:
		return "https://docs.google.com/document/d/" + id + "/edit"
	case KindSheet:
		return "https://docs.google.com/spreadsheets/d/" + id + "/edit"
	case KindSlides:
		return "https://docs.google.com/presentation/d/" + id + "/edit"
	}
	return "https://drive.google.com/file/d/" + id + "/view"
}

// EscapeQuery makes text safe between single quotes in a Drive search: the backslash first, then the
// quote.
func EscapeQuery(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, `\`, `\\`), `'`, `\'`)
}

// FindFolder answers the id of the oldest folder with this name, or "" when there is none.
func (c *Client) FindFolder(ctx context.Context, name string) (string, error) {
	query := url.Values{
		"q":         {"mimeType='" + MimeFolder + "' and name='" + EscapeQuery(name) + "' and trashed=false"},
		"orderBy":   {"createdTime"},
		"pageSize":  {"1"},
		fieldsParam: {"files(id)"},
	}
	var answer struct {
		Files []driveFile `json:"files"`
	}
	if err := c.getJSON(ctx, APIDrive, "/drive/v3/files", query, &answer); err != nil {
		return "", fmt.Errorf("look for the Drive folder %q: %w", name, err)
	}
	if len(answer.Files) == 0 {
		return "", nil
	}
	return answer.Files[0].ID, nil
}

// MakeFolder makes a folder in the top of the person's Drive and answers its id.
func (c *Client) MakeFolder(ctx context.Context, name string) (string, error) {
	meta := driveMeta{Name: name, MimeType: MimeFolder}
	var made driveFile
	if err := c.postJSON(ctx, APIDrive, "/drive/v3/files", url.Values{fieldsParam: {"id"}}, meta, &made); err != nil {
		return "", fmt.Errorf("make the Drive folder %q: %w", name, err)
	}
	if made.ID == "" {
		return "", errors.New("make the Drive folder: Google gave no id")
	}
	return made.ID, nil
}

// List answers the files of one kind that Marshal can see, newest change first, at most twenty. A
// folder is never listed.
func (c *Client) List(ctx context.Context, kind Kind) ([]File, error) {
	query := url.Values{
		"q":         {"trashed=false and " + kindFilter(kind)},
		"orderBy":   {"modifiedTime desc"},
		"pageSize":  {fmt.Sprint(listSize)},
		fieldsParam: {"files(" + fileFields + ")"},
	}
	var answer struct {
		Files []driveFile `json:"files"`
	}
	if err := c.getJSON(ctx, APIDrive, "/drive/v3/files", query, &answer); err != nil {
		return nil, fmt.Errorf("list the files Marshal made: %w", err)
	}
	files := make([]File, 0, len(answer.Files))
	for _, item := range answer.Files {
		files = append(files, fileOf(item))
	}
	return files, nil
}

// kindFilter is the part of a Drive search that picks one kind of file.
func kindFilter(kind Kind) string {
	switch kind {
	case KindDoc:
		return "mimeType='" + MimeDoc + "'"
	case KindSheet:
		return "mimeType='" + MimeSheet + "'"
	case KindSlides:
		return "mimeType='" + MimeSlides + "'"
	case KindFile:
		return "mimeType!='" + MimeFolder + "' and mimeType!='" + MimeDoc + "' and mimeType!='" +
			MimeSheet + "' and mimeType!='" + MimeSlides + "'"
	}
	return "mimeType!='" + MimeFolder + "'"
}

// driveMeta is what Marshal says about a new file.
type driveMeta struct {
	Name     string   `json:"name"`
	MimeType string   `json:"mimeType,omitempty"`
	Parents  []string `json:"parents,omitempty"`
}

func metaIn(folderID, name, mimeType string) driveMeta {
	meta := driveMeta{Name: name, MimeType: mimeType}
	if folderID != "" {
		meta.Parents = []string{folderID}
	}
	return meta
}

// CleanMediaType checks the type of an uploaded file and writes it back out the one safe way. It
// refuses anything that is not a plain "type/subtype" with parameters, and the types Google makes
// itself, which would turn the upload into something else.
func CleanMediaType(raw string) (string, error) {
	mediaType, params, err := mime.ParseMediaType(raw)
	if err != nil {
		return "", fmt.Errorf("read the file type %q: %w", raw, err)
	}
	if !strings.Contains(mediaType, "/") {
		return "", fmt.Errorf("the file type %q is not a type and a subtype", mediaType)
	}
	if strings.HasPrefix(mediaType, googleAppsPrefix) {
		return "", fmt.Errorf("the file type %q is one Google makes itself", mediaType)
	}
	clean := mime.FormatMediaType(mediaType, params)
	if clean == "" {
		return "", fmt.Errorf("the file type %q cannot be written", raw)
	}
	return clean, nil
}

// upload sends one file to Drive with its details in the first part and its content in the second.
func (c *Client) upload(ctx context.Context, meta driveMeta, mediaType string, media []byte) (File, error) {
	details, err := json.Marshal(meta)
	if err != nil {
		return File{}, fmt.Errorf("write the upload to Drive: %w", err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writePart(writer, "application/json; charset=UTF-8", details); err != nil {
		return File{}, err
	}
	if err := writePart(writer, mediaType, media); err != nil {
		return File{}, err
	}
	if err := writer.Close(); err != nil {
		return File{}, fmt.Errorf("write the upload to Drive: %w", err)
	}
	query := url.Values{"uploadType": {"multipart"}, fieldsParam: {fileFields}}
	var made driveFile
	send := request{
		api: APIDrive, method: http.MethodPost, target: c.endpoint(APIDrive, "/upload/drive/v3/files", query),
		contentType: "multipart/related; boundary=" + writer.Boundary(), body: body.Bytes(),
	}
	err = c.call(ctx, send, &made)
	if err != nil {
		return File{}, fmt.Errorf("save %q to Drive: %w", meta.Name, err)
	}
	return checkedFile(made)
}

func writePart(writer *multipart.Writer, contentType string, content []byte) error {
	header := textproto.MIMEHeader{}
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("write the upload to Drive: %w", err)
	}
	if _, err := part.Write(content); err != nil {
		return fmt.Errorf("write the upload to Drive: %w", err)
	}
	return nil
}

func checkedFile(made driveFile) (File, error) {
	if made.ID == "" {
		return File{}, errors.New("save the file to Drive: Google gave no id")
	}
	return fileOf(made), nil
}

// CreateDoc makes a Google Doc from HTML or plain text. Drive turns the upload into a document.
func (c *Client) CreateDoc(ctx context.Context, folderID, title, body string, isHTML bool) (File, error) {
	mediaType := textMedia
	if isHTML {
		mediaType = htmlMedia
	}
	return c.upload(ctx, metaIn(folderID, title, MimeDoc), mediaType, []byte(body))
}

// CreateSheet makes a Google Sheet from rows of cells. The first row is the header.
func (c *Client) CreateSheet(ctx context.Context, folderID, title string, rows [][]string) (File, error) {
	data, err := SheetCSV(rows)
	if err != nil {
		return File{}, err
	}
	return c.upload(ctx, metaIn(folderID, title, MimeSheet), csvMedia, data)
}

// CreateFile saves a plain file as it is, with no conversion. No type means plain text.
func (c *Client) CreateFile(ctx context.Context, folderID, name, mediaType, content string) (File, error) {
	if mediaType == "" {
		mediaType = textMedia
	}
	clean, err := CleanMediaType(mediaType)
	if err != nil {
		return File{}, err
	}
	return c.upload(ctx, metaIn(folderID, name, ""), clean, []byte(content))
}

// plainNumber is a negative number as a person writes it, which a spreadsheet does not take as a
// formula.
var plainNumber = regexp.MustCompile(`^-(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)

// SheetCSV writes rows as CSV for Drive to turn into a sheet. A cell a spreadsheet would run as a
// formula (one that starts with =, +, @, a tab or a carriage return, or with - and is not a plain
// number) gets a quote mark in front, so it stays text.
func SheetCSV(rows [][]string) ([]byte, error) {
	var out bytes.Buffer
	writer := csv.NewWriter(&out)
	for _, row := range rows {
		safe := make([]string, len(row))
		for i, cell := range row {
			safe[i] = neutralize(cell)
		}
		if err := writer.Write(safe); err != nil {
			return nil, fmt.Errorf("write the rows as CSV: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("write the rows as CSV: %w", err)
	}
	return out.Bytes(), nil
}

func neutralize(cell string) string {
	if cell == "" {
		return cell
	}
	switch cell[0] {
	case '=', '+', '@', '\t', '\r':
		return "'" + cell
	case '-':
		if !plainNumber.MatchString(cell) {
			return "'" + cell
		}
	}
	return cell
}

// ProbeDrive asks Drive for one file. It proves the Drive API is on and the token works, and makes
// nothing.
func (c *Client) ProbeDrive(ctx context.Context) error {
	query := url.Values{"pageSize": {"1"}, fieldsParam: {"files(id)"}}
	return c.getJSON(ctx, APIDrive, "/drive/v3/files", query, nil)
}

// ProbeAPI asks the Docs, Sheets or Slides API for a file that cannot exist. Any refusal about
// that file proves the API is on, so it answers nil. A refused token (401), an API that is off, or
// a Google that cannot be reached is an error.
func (c *Client) ProbeAPI(ctx context.Context, api API) error {
	path := map[API]string{
		APIDocs:   "/v1/documents/",
		APISheets: "/v4/spreadsheets/",
		APISlides: "/v1/presentations/",
	}[api]
	if path == "" {
		return fmt.Errorf("probe the %q API: there is no such API", api)
	}
	err := c.getJSON(ctx, api, path+probeID, nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	if apiErr.Unauthorized() || apiErr.NotEnabled() || apiErr.Status >= http.StatusInternalServerError {
		return err
	}
	return nil
}
