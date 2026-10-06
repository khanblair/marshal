package integrations

// This file owns the four Google file connections: Drive, Docs, Sheets and Slides. Each has its own
// consent and token (google.go), and all four use one Drive folder, named in Drive's settings. It
// never edits or deletes a file the person had. The REST calls are in integrations/googlefiles.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/oauth2"

	"github.com/khanblair/marshal/daemon/internal/integrations/googlefiles"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The ids of the four file connections, and the kinds their rows and tests are filed under.
const (
	GDriveID  = "gdrive"
	GDocsID   = "gdocs"
	GSheetsID = "gsheets"
	GSlidesID = "gslides"

	KindGDrive  = "drive"
	KindGDocs   = "docs"
	KindGSheets = "sheets"
	KindGSlides = "slides"
)

// defaultFolder is the Drive folder Marshal's files go in until a person names another.
const defaultFolder = "Marshal"

// What Marshal accepts to make. A longer title, a bigger file or a bigger sheet is refused with a
// sentence, so nothing is cut without the person knowing.
const (
	maxFolderChars  = 100
	maxTitleChars   = 200
	maxContentBytes = 2 << 20
	maxSheetRows    = 10_000
	maxSheetColumns = 100
	maxSlides       = 100
	maxBullets      = 50
)

// The names of the checks a file connection's test reports.
const (
	CheckAccess = "Access"
	CheckDrive  = "Drive"
	CheckFolder = "Folder"
	CheckDocs   = "Docs"
	CheckSheets = "Sheets"
	CheckSlides = "Slides"
)

// driveConfig is Drive's settings: the name of the folder Marshal makes its files in.
type driveConfig struct {
	Folder string `json:"folder"`
}

// GoogleAccessError says which Google connection could not be used, so a refusal can name it. It
// wraps ErrNotConnected, ErrNoGoogleClient or ErrNeedsReconnect.
type GoogleAccessError struct {
	ID  string
	Err error
}

func (e *GoogleAccessError) Error() string { return e.Err.Error() }

func (e *GoogleAccessError) Unwrap() error { return e.Err }

// SaveGoogleDrive saves the name of the folder Marshal makes its files in. The folder is made the
// first time a file is saved, so nothing is sent to Google here.
func (s *Service) SaveGoogleDrive(ctx context.Context, req protocol.SaveGoogleDriveRequest) error {
	name := strings.TrimSpace(req.Folder)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxFolderChars || strings.ContainsFunc(name, unicode.IsControl) {
		return protocol.InvalidArgument("The folder name must be 1 to 100 characters, with no line breaks or other hidden characters.")
	}
	raw, err := json.Marshal(driveConfig{Folder: name})
	if err != nil {
		return fmt.Errorf("write Google Drive's settings: %w", err)
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertIntegration(ctx, db.UpsertIntegrationParams{
			ID: GDriveID, Kind: KindGDrive, ConfigJSON: string(raw), KeychainRef: GDriveID,
		})
	})
	if err != nil {
		return fmt.Errorf("save Google Drive's settings: %w", err)
	}
	s.forgetFolders()
	return nil
}

// driveFolder is the name of Marshal's folder: the one saved under Drive, or "Marshal". All four
// connections use it, whether or not Drive itself is connected.
func (s *Service) driveFolder(ctx context.Context) (string, error) {
	var name string
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetIntegration(ctx, GDriveID)
		if errors.Is(err, sql.ErrNoRows) || err == nil && row.ConfigJSON == "" {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read Google Drive's settings: %w", err)
		}
		var config driveConfig
		if err := json.Unmarshal([]byte(row.ConfigJSON), &config); err != nil {
			return fmt.Errorf("read Google Drive's settings: %w", err)
		}
		name = strings.TrimSpace(config.Folder)
		return nil
	})
	if err != nil {
		return "", err
	}
	if name == "" {
		name = defaultFolder
	}
	return name, nil
}

// filesClient builds a client from one connection's token, refreshing it first. The error names the
// connection when it is not connected or has to be connected again. The grant it answers names the
// access the client acts with.
func (s *Service) filesClient(ctx context.Context, id string) (*googlefiles.Client, string, error) {
	fresh, err := s.freshGoogleToken(ctx, id)
	switch {
	case errors.Is(err, ErrNotConnected), errors.Is(err, ErrNoGoogleClient), errors.Is(err, ErrNeedsReconnect):
		return nil, "", &GoogleAccessError{ID: id, Err: err}
	case err != nil:
		return nil, "", err
	}
	httpClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(fresh))
	return googlefiles.New(httpClient, s.filesBase), grantOf(fresh.RefreshToken), nil
}

// forgetFolders drops the folder ids found so far, after a connection or the folder's name changes.
func (s *Service) forgetFolders() {
	s.google.folderMu.Lock()
	defer s.google.folderMu.Unlock()
	s.google.folders = nil
}

// marshalFolder answers the id of Marshal's folder, finding it or making it. It asks Google once per
// grant and name, and uses the answer until a connection changes. again skips what was found, for a
// folder that has since been deleted.
func (s *Service) marshalFolder(ctx context.Context, client *googlefiles.Client, grant string, again bool) (string, error) {
	name, err := s.driveFolder(ctx)
	if err != nil {
		return "", err
	}
	key := grant + "\x00" + name
	s.google.folderMu.Lock()
	defer s.google.folderMu.Unlock()
	if id, found := s.google.folders[key]; found && !again {
		return id, nil
	}
	id, err := client.FindFolder(ctx, name)
	if err == nil && id == "" {
		id, err = client.MakeFolder(ctx, name)
	}
	if err != nil {
		return "", err
	}
	if s.google.folders == nil {
		s.google.folders = map[string]string{}
	}
	s.google.folders[key] = id
	return id, nil
}

// saveInFolder makes one file in Marshal's folder with one connection's token. A "not found" from
// Google may mean the folder was deleted since it was found, so the folder is looked for again, once.
func (s *Service) saveInFolder(ctx context.Context, id string, build func(*googlefiles.Client, string) (googlefiles.File, error)) (protocol.GoogleFile, error) {
	client, grant, err := s.filesClient(ctx, id)
	if err != nil {
		return protocol.GoogleFile{}, err
	}
	folder, err := s.marshalFolder(ctx, client, grant, false)
	if err != nil {
		return protocol.GoogleFile{}, s.filesError(id, err)
	}
	file, err := build(client, folder)
	var apiErr *googlefiles.APIError
	if errors.As(err, &apiErr) && apiErr.NotFound() {
		if folder, err = s.marshalFolder(ctx, client, grant, true); err == nil {
			file, err = build(client, folder)
		}
	}
	if err != nil {
		return protocol.GoogleFile{}, s.filesError(id, err)
	}
	return wireFile(file), nil
}

// wireFile turns a file Google answered into the shape a screen reads.
func wireFile(file googlefiles.File) protocol.GoogleFile {
	out := protocol.GoogleFile{ID: file.ID, Name: file.Name, Kind: wireKind(file.Kind()), URL: file.URL}
	if !file.ModifiedAt.IsZero() {
		modified := protocol.NewTimestamp(file.ModifiedAt)
		out.ModifiedAt = &modified
	}
	return out
}

func wireKind(kind googlefiles.Kind) protocol.GoogleFileKind {
	switch kind {
	case googlefiles.KindDoc:
		return protocol.GoogleFileKindDoc
	case googlefiles.KindSheet:
		return protocol.GoogleFileKindSheet
	case googlefiles.KindSlides:
		return protocol.GoogleFileKindSlides
	}
	return protocol.GoogleFileKindFile
}

// apiTitle is the name a person reads for one of the four APIs.
func apiTitle(api googlefiles.API) string {
	switch api {
	case googlefiles.APIDocs:
		return "Google Docs"
	case googlefiles.APISheets:
		return "Google Sheets"
	case googlefiles.APISlides:
		return "Google Slides"
	}
	return "Google Drive"
}

// filesError turns what Google answered into an error a person can act on. A token Google refuses
// means the connection has to be made again, an API that is turned off says which, and the rest say
// that Google is busy or could not be reached.
func (s *Service) filesError(id string, err error) error {
	var apiErr *googlefiles.APIError
	if !errors.As(err, &apiErr) {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return err
		case errors.Is(err, googlefiles.ErrUnreachable):
			return protocol.Unavailable("Marshal could not reach Google. Check this computer's connection, then try again.").WithCause(err)
		case errors.Is(err, googlefiles.ErrTooLarge):
			return protocol.Refused("That file is too big for Marshal to read.").WithCause(err)
		}
		return err
	}
	switch {
	case apiErr.Unauthorized():
		return &GoogleAccessError{ID: id, Err: fmt.Errorf("%w: %v", ErrNeedsReconnect, err)}
	case apiErr.NotEnabled():
		return protocol.Refused(turnOnSentence(apiErr.API)).WithCause(err)
	case apiErr.Status == http.StatusTooManyRequests || apiErr.Status >= http.StatusInternalServerError:
		return protocol.Unavailable("Google is busy right now. Try again in a moment.").WithCause(err)
	}
	return protocol.Refused(fmt.Sprintf("Google would not do that. Reconnect %s in Settings, and leave every box ticked.", GoogleName(id))).
		WithCause(err)
}

func turnOnSentence(api googlefiles.API) string {
	return fmt.Sprintf("Turn on the %s API in the Google Cloud project Marshal signs in with.", apiTitle(api))
}

// checkName checks a title or a file name: 1 to 200 characters once trimmed.
func checkName(name, what string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxTitleChars {
		return "", protocol.InvalidArgument(fmt.Sprintf("The %s must be 1 to %d characters.", what, maxTitleChars))
	}
	return name, nil
}

func tooBig(what string) error {
	return protocol.InvalidArgument(fmt.Sprintf("That %s is too big. Marshal saves up to 2 MB at a time.", what))
}

// CreateGoogleDoc makes a Google Doc in Marshal's folder from HTML or plain text. Google turns
// headings, lists, links and tables in the HTML into the document's own formatting.
func (s *Service) CreateGoogleDoc(ctx context.Context, req protocol.CreateGoogleDocRequest) (protocol.GoogleFile, error) {
	title, err := checkName(req.Title, "title")
	if err != nil {
		return protocol.GoogleFile{}, err
	}
	switch {
	case req.HTML != "" && req.Text != "":
		return protocol.GoogleFile{}, protocol.InvalidArgument("Send the document as HTML or as plain text, not both.")
	case req.HTML == "" && req.Text == "":
		return protocol.GoogleFile{}, protocol.InvalidArgument("Send the document's words as HTML or as plain text.")
	}
	body, isHTML := req.Text, false
	if req.HTML != "" {
		body, isHTML = req.HTML, true
	}
	if len(body) > maxContentBytes {
		return protocol.GoogleFile{}, tooBig("document")
	}
	return s.saveInFolder(ctx, GDocsID, func(c *googlefiles.Client, folder string) (googlefiles.File, error) {
		return c.CreateDoc(ctx, folder, title, body, isHTML)
	})
}

// CreateGoogleSheet makes a Google Sheet in Marshal's folder. The first row is the header.
func (s *Service) CreateGoogleSheet(ctx context.Context, req protocol.CreateGoogleSheetRequest) (protocol.GoogleFile, error) {
	title, err := checkName(req.Title, "title")
	if err != nil {
		return protocol.GoogleFile{}, err
	}
	if err := checkRows(req.Rows); err != nil {
		return protocol.GoogleFile{}, err
	}
	return s.saveInFolder(ctx, GSheetsID, func(c *googlefiles.Client, folder string) (googlefiles.File, error) {
		return c.CreateSheet(ctx, folder, title, req.Rows)
	})
}

// checkRows holds a sheet to the rows, the columns and the size Marshal accepts.
func checkRows(rows [][]string) error {
	if len(rows) == 0 {
		return protocol.InvalidArgument("Add at least one row to the sheet.")
	}
	if len(rows) > maxSheetRows {
		return protocol.InvalidArgument("A sheet can have up to 10,000 rows.")
	}
	size := 0
	for _, row := range rows {
		if len(row) > maxSheetColumns {
			return protocol.InvalidArgument("A sheet can have up to 100 columns.")
		}
		for _, cell := range row {
			size += len(cell) + 1
		}
	}
	if size > maxContentBytes {
		return tooBig("sheet")
	}
	return nil
}

// CreateGoogleSlides makes a Google Slides presentation in Marshal's folder, one slide per entry.
func (s *Service) CreateGoogleSlides(ctx context.Context, req protocol.CreateGoogleSlidesRequest) (protocol.GoogleFile, error) {
	title, err := checkName(req.Title, "title")
	if err != nil {
		return protocol.GoogleFile{}, err
	}
	slides, err := checkSlides(req.Slides)
	if err != nil {
		return protocol.GoogleFile{}, err
	}
	return s.saveInFolder(ctx, GSlidesID, func(c *googlefiles.Client, folder string) (googlefiles.File, error) {
		return c.CreateSlides(ctx, folder, title, slides)
	})
}

// checkSlides holds a deck to the slides, the bullets and the size Marshal accepts.
func checkSlides(in []protocol.GoogleSlide) ([]googlefiles.Slide, error) {
	if len(in) < 1 || len(in) > maxSlides {
		return nil, protocol.InvalidArgument("A presentation needs 1 to 100 slides.")
	}
	size := 0
	slides := make([]googlefiles.Slide, 0, len(in))
	for _, slide := range in {
		if len(slide.Bullets) > maxBullets {
			return nil, protocol.InvalidArgument("A slide can have up to 50 lines under its title.")
		}
		size += len(slide.Title)
		for _, bullet := range slide.Bullets {
			size += len(bullet)
		}
		slides = append(slides, googlefiles.Slide{Title: slide.Title, Bullets: slide.Bullets})
	}
	if size > maxContentBytes {
		return nil, tooBig("presentation")
	}
	return slides, nil
}

// UploadGoogleFile saves a plain file to Marshal's folder as it is, with no conversion.
func (s *Service) UploadGoogleFile(ctx context.Context, req protocol.UploadGoogleFileRequest) (protocol.GoogleFile, error) {
	name, err := checkName(req.Name, "name")
	if err != nil {
		return protocol.GoogleFile{}, err
	}
	if len(req.Content) > maxContentBytes {
		return protocol.GoogleFile{}, tooBig("file")
	}
	// No type means plain text, which the client fills in.
	mediaType := strings.TrimSpace(req.MimeType)
	if mediaType != "" {
		if _, err := googlefiles.CleanMediaType(mediaType); err != nil {
			return protocol.GoogleFile{}, protocol.InvalidArgument("Marshal cannot save a file of that type. Use a type like text/plain or text/markdown.").WithCause(err)
		}
	}
	return s.saveInFolder(ctx, GDriveID, func(c *googlefiles.Client, folder string) (googlefiles.File, error) {
		return c.CreateFile(ctx, folder, name, mediaType, req.Content)
	})
}

// GoogleFiles lists what Marshal made, newest first, at most twenty. kind picks one sort of file,
// with the matching connection's token; no kind lists every sort but folders, with Drive's.
func (s *Service) GoogleFiles(ctx context.Context, kind protocol.GoogleFileKind) (protocol.GoogleFiles, error) {
	id, filter := GDriveID, googlefiles.KindAny
	switch kind {
	case "":
	case protocol.GoogleFileKindDoc:
		id, filter = GDocsID, googlefiles.KindDoc
	case protocol.GoogleFileKindSheet:
		id, filter = GSheetsID, googlefiles.KindSheet
	case protocol.GoogleFileKindSlides:
		id, filter = GSlidesID, googlefiles.KindSlides
	case protocol.GoogleFileKindFile:
		filter = googlefiles.KindFile
	default:
		return protocol.GoogleFiles{}, protocol.InvalidArgument("Choose doc, sheet, slides or file.").With("kind", string(kind))
	}
	client, _, err := s.filesClient(ctx, id)
	if err != nil {
		return protocol.GoogleFiles{}, err
	}
	folder, err := s.driveFolder(ctx)
	if err != nil {
		return protocol.GoogleFiles{}, err
	}
	found, err := client.List(ctx, filter)
	if err != nil {
		return protocol.GoogleFiles{}, s.filesError(id, err)
	}
	out := protocol.GoogleFiles{Files: make([]protocol.GoogleFile, 0, len(found)), Folder: folder}
	for _, file := range found {
		out.Files = append(out.Files, wireFile(file))
	}
	return out, nil
}

// ReadGoogleLink reads the Google Doc, Sheet or Slides presentation a link points to, into markdown.
// Only the file's id is taken from the link: Marshal never opens the address itself. It needs the
// matching connection, because each reads through its own.
func (s *Service) ReadGoogleLink(ctx context.Context, req protocol.ReadGoogleLinkRequest) (protocol.GoogleLinkContent, error) {
	kind, fileID, ok := googlefiles.ParseLink(req.URL)
	if !ok {
		return protocol.GoogleLinkContent{}, protocol.InvalidArgument("Paste the address of a Google Doc, Sheet or Slides presentation.")
	}
	id := GDocsID
	read := func(c *googlefiles.Client) (googlefiles.Content, error) { return c.ReadDoc(ctx, fileID) }
	switch kind {
	case googlefiles.KindSheet:
		id, read = GSheetsID, func(c *googlefiles.Client) (googlefiles.Content, error) { return c.ReadSheet(ctx, fileID) }
	case googlefiles.KindSlides:
		id, read = GSlidesID, func(c *googlefiles.Client) (googlefiles.Content, error) { return c.ReadSlides(ctx, fileID) }
	}
	client, _, err := s.filesClient(ctx, id)
	if err != nil {
		return protocol.GoogleLinkContent{}, err
	}
	content, err := read(client)
	if err != nil {
		return protocol.GoogleLinkContent{}, s.readError(id, err)
	}
	return protocol.GoogleLinkContent{
		Kind: wireKind(kind), ID: fileID, Title: content.Title, URL: googlefiles.ViewURL(kind, fileID),
		Markdown: content.Markdown, Truncated: content.Truncated,
	}, nil
}

// readError is filesError for reading a file by link, where a file that is missing, private, or not a
// Google file is the person's own link to fix.
func (s *Service) readError(id string, err error) error {
	var apiErr *googlefiles.APIError
	if errors.As(err, &apiErr) && !apiErr.Unauthorized() && !apiErr.NotEnabled() &&
		apiErr.Status >= http.StatusBadRequest && apiErr.Status < http.StatusInternalServerError &&
		apiErr.Status != http.StatusTooManyRequests {
		return protocol.Refused("Marshal could not open that file. Check the link, and that the Google account you connected can open it.").WithCause(err)
	}
	return s.filesError(id, err)
}
