package protocol

import "slices"

// The wire shapes of the Google file connections: Drive, Docs, Sheets and Slides. Each is its own
// connection (ids "gdrive", "gdocs", "gsheets", "gslides") with its own consent and token. Marshal
// makes new files in a folder of the person's Drive, and reads an existing file the person points
// it to with a link.

// GoogleFileKind is what sort of Google file something is.
type GoogleFileKind string

const (
	// GoogleFileKindDoc is a Google Doc.
	GoogleFileKindDoc GoogleFileKind = "doc"
	// GoogleFileKindSheet is a Google Sheet.
	GoogleFileKindSheet GoogleFileKind = "sheet"
	// GoogleFileKindSlides is a Google Slides presentation.
	GoogleFileKindSlides GoogleFileKind = "slides"
	// GoogleFileKindFile is any other file kept in Drive, such as a text or markdown file.
	GoogleFileKindFile GoogleFileKind = "file"
)

// GoogleFileKindValues lists every kind of Google file.
func GoogleFileKindValues() []GoogleFileKind {
	return []GoogleFileKind{
		GoogleFileKindDoc, GoogleFileKindSheet, GoogleFileKindSlides, GoogleFileKindFile,
	}
}

// Valid reports whether k is a kind of Google file.
func (k GoogleFileKind) Valid() bool { return slices.Contains(GoogleFileKindValues(), k) }

// GoogleFile is one file in the person's Google Drive.
type GoogleFile struct {
	// ID is Google's own id for the file.
	ID string `json:"id"`
	// Name is the file's title.
	Name string `json:"name"`
	// Kind is what sort of file it is.
	Kind GoogleFileKind `json:"kind"`
	// URL opens the file in Google.
	URL string `json:"url"`
	// ModifiedAt is when the file last changed. Absent when Google did not say.
	ModifiedAt *Timestamp `json:"modifiedAt,omitempty"`
}

// GoogleFiles is the answer to GET /v1/google/files: the files Marshal made, newest first.
type GoogleFiles struct {
	// Files is never null.
	Files []GoogleFile `json:"files"`
	// Folder is the name of the Drive folder Marshal puts new files in.
	Folder string `json:"folder"`
}

// SaveGoogleDriveRequest is the body that saves Google Drive's one setting.
type SaveGoogleDriveRequest struct {
	// Folder is the name of the Drive folder Marshal puts the files it makes in. Marshal makes the
	// folder the first time it needs it.
	Folder string `json:"folder"`
}

// CreateGoogleDocRequest is the body of POST /v1/google/docs. Exactly one of HTML and Text is set.
type CreateGoogleDocRequest struct {
	// Title is the document's name.
	Title string `json:"title"`
	// HTML is the document's body as HTML. Google turns headings, lists, links and tables into
	// the document's own formatting.
	HTML string `json:"html,omitempty"`
	// Text is the document's body as plain text.
	Text string `json:"text,omitempty"`
}

// CreateGoogleSheetRequest is the body of POST /v1/google/sheets.
type CreateGoogleSheetRequest struct {
	// Title is the spreadsheet's name.
	Title string `json:"title"`
	// Rows are the cells, row by row. The first row is the header.
	Rows [][]string `json:"rows"`
}

// GoogleSlide is one slide of a presentation Marshal makes.
type GoogleSlide struct {
	// Title is the slide's heading.
	Title string `json:"title"`
	// Bullets are the lines under the heading.
	Bullets []string `json:"bullets"`
}

// CreateGoogleSlidesRequest is the body of POST /v1/google/slides.
type CreateGoogleSlidesRequest struct {
	// Title is the presentation's name.
	Title string `json:"title"`
	// Slides are the slides in order. At least one.
	Slides []GoogleSlide `json:"slides"`
}

// UploadGoogleFileRequest is the body of POST /v1/google/drive/files: a plain file saved to Drive
// as it is, with no conversion.
type UploadGoogleFileRequest struct {
	// Name is the file's name, with its extension.
	Name string `json:"name"`
	// Content is the file's text.
	Content string `json:"content"`
	// MimeType is the file's type, such as "text/markdown". Empty means plain text.
	MimeType string `json:"mimeType,omitempty"`
}

// ReadGoogleLinkRequest is the body of POST /v1/google/read.
type ReadGoogleLinkRequest struct {
	// URL is the address of a Google Doc, Sheet or Slides presentation, as copied from the browser.
	URL string `json:"url"`
}

// GoogleLinkContent is a Google file read from a link, turned into markdown a card's note can hold.
type GoogleLinkContent struct {
	// Kind is what sort of file the link points to: a doc, a sheet, or slides.
	Kind GoogleFileKind `json:"kind"`
	// ID is Google's own id for the file.
	ID string `json:"id"`
	// Title is the file's name.
	Title string `json:"title"`
	// URL opens the file in Google.
	URL string `json:"url"`
	// Markdown is the file's content: a doc's text, a sheet's first tab as a table, or a slide
	// outline.
	Markdown string `json:"markdown"`
	// Truncated says the file was longer than Marshal reads, so Markdown is only the start.
	Truncated bool `json:"truncated"`
}
