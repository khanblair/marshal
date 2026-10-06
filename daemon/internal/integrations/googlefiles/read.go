package googlefiles

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxMarkdownBytes is the most markdown one read answers. A longer file is cut on a line.
	MaxMarkdownBytes = 200_000
	// sheetRange is the part of a sheet's first tab that is read.
	sheetRange = "A1:Z200"
	// sheetRows is how many rows that range holds. A read that fills it may have left rows out.
	sheetRows = 200
	// untitledSlide stands in for a slide that has no title.
	untitledSlide = "Untitled"
)

// Content is a Google file read into markdown.
type Content struct {
	// Title is the file's name.
	Title string
	// Markdown is what the file holds.
	Markdown string
	// Truncated says the file was longer than Marshal reads, so Markdown is only the start.
	Truncated bool
}

// ReadDoc reads a Google Doc into markdown.
func (c *Client) ReadDoc(ctx context.Context, id string) (Content, error) {
	var doc docJSON
	if err := c.getJSON(ctx, APIDocs, "/v1/documents/"+url.PathEscape(id), nil, &doc); err != nil {
		return Content{}, fmt.Errorf("read the Google Doc: %w", err)
	}
	text, cut := capMarkdown(docMarkdown(doc))
	return Content{Title: doc.Title, Markdown: text, Truncated: cut}, nil
}

// ReadSheet reads the first tab of a Google Sheet into one markdown table. The first row is the
// header.
func (c *Client) ReadSheet(ctx context.Context, id string) (Content, error) {
	var meta struct {
		Properties struct {
			Title string `json:"title"`
		} `json:"properties"`
		Sheets []struct {
			Properties struct {
				Title string `json:"title"`
			} `json:"properties"`
		} `json:"sheets"`
	}
	path := "/v4/spreadsheets/" + url.PathEscape(id)
	query := url.Values{fieldsParam: {"properties.title,sheets.properties.title"}}
	if err := c.getJSON(ctx, APISheets, path, query, &meta); err != nil {
		return Content{}, fmt.Errorf("read the Google Sheet: %w", err)
	}
	content := Content{Title: meta.Properties.Title}
	if len(meta.Sheets) == 0 {
		return content, nil
	}
	var values struct {
		Values [][]any `json:"values"`
	}
	tab := "'" + strings.ReplaceAll(meta.Sheets[0].Properties.Title, "'", "''") + "'!" + sheetRange
	if err := c.getJSON(ctx, APISheets, path+"/values/"+url.PathEscape(tab), nil, &values); err != nil {
		return Content{}, fmt.Errorf("read the Google Sheet's first tab: %w", err)
	}
	text, cut := capMarkdown(sheetMarkdown(values.Values))
	content.Markdown, content.Truncated = text, cut || len(values.Values) >= sheetRows
	return content, nil
}

// ReadSlides reads a Google Slides presentation into an outline: a heading per slide, then a bullet
// per line of text.
func (c *Client) ReadSlides(ctx context.Context, id string) (Content, error) {
	var deck presentationJSON
	if err := c.getJSON(ctx, APISlides, "/v1/presentations/"+url.PathEscape(id), nil, &deck); err != nil {
		return Content{}, fmt.Errorf("read the Google Slides presentation: %w", err)
	}
	text, cut := capMarkdown(slidesMarkdown(deck))
	return Content{Title: deck.Title, Markdown: text, Truncated: cut}, nil
}

// capMarkdown cuts text that is longer than Marshal reads, on a line boundary when there is one.
func capMarkdown(text string) (string, bool) {
	if len(text) <= MaxMarkdownBytes {
		return text, false
	}
	cut := text[:MaxMarkdownBytes]
	if end := strings.LastIndexByte(cut, '\n'); end > 0 {
		return strings.TrimRight(cut[:end], "\n"), true
	}
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut, true
}

// docJSON is the part of a Docs document that becomes markdown.
type docJSON struct {
	Title string `json:"title"`
	Body  struct {
		Content []docElement `json:"content"`
	} `json:"body"`
}

type docElement struct {
	Paragraph *docParagraph `json:"paragraph"`
	Table     *docTable     `json:"table"`
}

type docParagraph struct {
	Elements []struct {
		TextRun *docRun `json:"textRun"`
	} `json:"elements"`
	Style struct {
		Named string `json:"namedStyleType"`
	} `json:"paragraphStyle"`
	Bullet *struct {
		NestingLevel int `json:"nestingLevel"`
	} `json:"bullet"`
}

type docRun struct {
	Content string `json:"content"`
	Style   struct {
		Bold   bool `json:"bold"`
		Italic bool `json:"italic"`
		Link   *struct {
			URL string `json:"url"`
		} `json:"link"`
	} `json:"textStyle"`
}

type docTable struct {
	Rows []struct {
		Cells []struct {
			Content []docElement `json:"content"`
		} `json:"tableCells"`
	} `json:"tableRows"`
}

// span is a stretch of text that is all in one style.
type span struct {
	text         string
	bold, italic bool
	link         string
}

// docMarkdown turns a document into markdown: headings, bullets, bold, italic, links, and tables.
// An empty paragraph adds nothing, so a document does not come out with runs of blank lines.
func docMarkdown(doc docJSON) string {
	var out mdWriter
	for _, element := range doc.Body.Content {
		switch {
		case element.Paragraph != nil:
			writeParagraph(&out, element.Paragraph)
		case element.Table != nil:
			writeTable(&out, element.Table)
		}
	}
	return out.String()
}

func writeParagraph(out *mdWriter, p *docParagraph) {
	inline := p.Bullet != nil || strings.HasPrefix(p.Style.Named, "HEADING_")
	text := strings.TrimSpace(renderSpans(paragraphSpans(p), inline))
	if text == "" {
		return
	}
	if p.Bullet != nil {
		out.item(strings.Repeat("  ", max(p.Bullet.NestingLevel, 0)) + "- " + text)
		return
	}
	if level := headingLevel(p.Style.Named); level > 0 {
		text = strings.Repeat("#", level) + " " + text
	}
	out.block(text)
}

// headingLevel is 1 to 6 for HEADING_1 to HEADING_6, and 0 for any other paragraph style.
func headingLevel(named string) int {
	level, found := strings.CutPrefix(named, "HEADING_")
	if !found || len(level) != 1 || level[0] < '1' || level[0] > '6' {
		return 0
	}
	return int(level[0] - '0')
}

// paragraphSpans reads a paragraph's runs, joining neighbours that share a style so a bold phrase
// that Docs stored in three runs is written once.
func paragraphSpans(p *docParagraph) []span {
	var spans []span
	for _, element := range p.Elements {
		run := element.TextRun
		if run == nil || run.Content == "" {
			continue
		}
		next := span{text: cleanText(run.Content), bold: run.Style.Bold, italic: run.Style.Italic}
		if run.Style.Link != nil {
			next.link = run.Style.Link.URL
		}
		if last := len(spans) - 1; last >= 0 && sameStyle(spans[last], next) {
			spans[last].text += next.text
			continue
		}
		spans = append(spans, next)
	}
	if len(spans) > 0 {
		last := &spans[len(spans)-1]
		last.text = strings.TrimRight(last.text, "\n")
	}
	return spans
}

func sameStyle(a, b span) bool { return a.bold == b.bold && a.italic == b.italic && a.link == b.link }

// cleanText drops the control characters Docs puts in text, and keeps a soft line break as one.
func cleanText(text string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\v':
			return '\n'
		case r == '\n' || r == '\t' || !unicode.IsControl(r):
			return r
		}
		return -1
	}, text)
}

// renderSpans writes spans as markdown. A style goes around the words and never around the spaces
// beside them, because markdown does not read "** bold **". inline turns a line break inside the
// paragraph into a space, for a heading or a bullet, which are one line.
func renderSpans(spans []span, inline bool) string {
	var out strings.Builder
	for _, item := range spans {
		text := item.text
		if inline {
			text = strings.ReplaceAll(text, "\n", " ")
		} else {
			text = strings.ReplaceAll(text, "\n", "  \n")
		}
		core := strings.TrimSpace(text)
		if core == "" {
			out.WriteString(text)
			continue
		}
		lead := text[:strings.Index(text, core)]
		trail := text[len(lead)+len(core):]
		if item.link != "" {
			core = "[" + core + "](" + item.link + ")"
		}
		if item.italic {
			core = "*" + core + "*"
		}
		if item.bold {
			core = "**" + core + "**"
		}
		out.WriteString(lead + core + trail)
	}
	return out.String()
}

func writeTable(out *mdWriter, table *docTable) {
	rows := make([][]string, 0, len(table.Rows))
	for _, row := range table.Rows {
		cells := make([]string, 0, len(row.Cells))
		for _, cell := range row.Cells {
			cells = append(cells, cellText(cell.Content))
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return
	}
	out.block(markdownTable(rows))
}

// cellText is what one table cell says, as one line.
func cellText(content []docElement) string {
	var parts []string
	for _, element := range content {
		if element.Paragraph == nil {
			continue
		}
		if text := strings.TrimSpace(renderSpans(paragraphSpans(element.Paragraph), true)); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

// markdownTable writes rows as a markdown table whose header is the first row. Rows that are shorter
// than the longest are filled out with empty cells.
func markdownTable(rows [][]string) string {
	width := 0
	for _, row := range rows {
		width = max(width, len(row))
	}
	if width == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows)+1)
	for i, row := range rows {
		cells := make([]string, width)
		for col := range cells {
			if col < len(row) {
				cells[col] = escapeCell(row[col])
			}
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
		if i == 0 {
			lines = append(lines, "|"+strings.Repeat(" --- |", width))
		}
	}
	return strings.Join(lines, "\n")
}

// escapeCell makes a cell safe inside a markdown table: a pipe is escaped and a line break becomes
// a space.
func escapeCell(cell string) string {
	cell = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "|", `\|`).Replace(cell)
	return strings.TrimSpace(cell)
}

// sheetMarkdown writes a sheet's cells as one markdown table. Nothing at all is an empty string.
func sheetMarkdown(values [][]any) string {
	rows := make([][]string, 0, len(values))
	for _, row := range values {
		cells := make([]string, 0, len(row))
		for _, cell := range row {
			if cell == nil {
				cells = append(cells, "")
				continue
			}
			cells = append(cells, fmt.Sprint(cell))
		}
		rows = append(rows, cells)
	}
	return markdownTable(rows)
}

// mdWriter puts markdown blocks together: one blank line between blocks, and none between the lines
// of a list.
type mdWriter struct {
	lines  []string
	inList bool
}

func (w *mdWriter) block(text string) {
	if len(w.lines) > 0 {
		w.lines = append(w.lines, "")
	}
	w.lines = append(w.lines, text)
	w.inList = false
}

func (w *mdWriter) item(text string) {
	if len(w.lines) > 0 && !w.inList {
		w.lines = append(w.lines, "")
	}
	w.lines = append(w.lines, text)
	w.inList = true
}

func (w *mdWriter) String() string { return strings.Join(w.lines, "\n") }

// presentationJSON is the part of a Slides presentation that becomes an outline.
type presentationJSON struct {
	Title  string `json:"title"`
	Slides []struct {
		PageElements []pageElement `json:"pageElements"`
	} `json:"slides"`
}

type pageElement struct {
	Shape *struct {
		Placeholder *placeholder `json:"placeholder"`
		Text        *slideText   `json:"text"`
	} `json:"shape"`
	Table *struct {
		Rows []struct {
			Cells []struct {
				Text *slideText `json:"text"`
			} `json:"tableCells"`
		} `json:"tableRows"`
	} `json:"table"`
	Group *struct {
		Children []pageElement `json:"children"`
	} `json:"elementGroup"`
}

type placeholder struct {
	Type string `json:"type"`
}

type slideText struct {
	Elements []struct {
		Run *struct {
			Content string `json:"content"`
		} `json:"textRun"`
		Auto *struct {
			Content string `json:"content"`
		} `json:"autoText"`
	} `json:"textElements"`
}

// lines are the lines of text in a shape or a cell, without the empty ones.
func (t *slideText) lines() []string {
	if t == nil {
		return nil
	}
	var all strings.Builder
	for _, element := range t.Elements {
		switch {
		case element.Run != nil:
			all.WriteString(element.Run.Content)
		case element.Auto != nil:
			all.WriteString(element.Auto.Content)
		}
	}
	var out []string
	for _, line := range strings.FieldsFunc(cleanText(all.String()), func(r rune) bool { return r == '\n' }) {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// slidesMarkdown writes each slide as a numbered heading, then a bullet for each line of its text.
// A title placeholder is the heading, and every other line is a bullet.
func slidesMarkdown(deck presentationJSON) string {
	var out mdWriter
	for i, slide := range deck.Slides {
		var title string
		var bullets []string
		collectSlideText(slide.PageElements, &title, &bullets)
		if title == "" {
			title = untitledSlide
		}
		lines := []string{fmt.Sprintf("## %d. %s", i+1, title)}
		for _, bullet := range bullets {
			lines = append(lines, "- "+bullet)
		}
		out.block(strings.Join(lines, "\n"))
	}
	return out.String()
}

func collectSlideText(elements []pageElement, title *string, bullets *[]string) {
	for _, element := range elements {
		switch {
		case element.Shape != nil && isTitle(element.Shape.Placeholder):
			if *title == "" {
				*title = strings.Join(element.Shape.Text.lines(), " ")
				continue
			}
			*bullets = append(*bullets, element.Shape.Text.lines()...)
		case element.Shape != nil:
			*bullets = append(*bullets, element.Shape.Text.lines()...)
		case element.Table != nil:
			for _, row := range element.Table.Rows {
				for _, cell := range row.Cells {
					*bullets = append(*bullets, cell.Text.lines()...)
				}
			}
		case element.Group != nil:
			collectSlideText(element.Group.Children, title, bullets)
		}
	}
}

func isTitle(p *placeholder) bool {
	return p != nil && (p.Type == "TITLE" || p.Type == "CENTERED_TITLE")
}
