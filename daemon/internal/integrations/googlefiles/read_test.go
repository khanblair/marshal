package googlefiles

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// docFixture is a document with a heading, styled text, a link, nested bullets, an empty paragraph,
// a soft line break, and a table with a pipe in a cell.
const docFixture = `{"title":"Launch plan","body":{"content":[
 {"sectionBreak":{}},
 {"paragraph":{"paragraphStyle":{"namedStyleType":"HEADING_1"},"elements":[{"textRun":{"content":"Launch plan\n"}}]}},
 {"paragraph":{"paragraphStyle":{"namedStyleType":"NORMAL_TEXT"},"elements":[
   {"textRun":{"content":"This is ","textStyle":{}}},
   {"textRun":{"content":"very ","textStyle":{"bold":true}}},
   {"textRun":{"content":"important","textStyle":{"bold":true}}},
   {"textRun":{"content":" and ","textStyle":{}}},
   {"textRun":{"content":"soft","textStyle":{"italic":true}}},
   {"textRun":{"content":" see ","textStyle":{}}},
   {"textRun":{"content":"the site","textStyle":{"link":{"url":"https://example.com/a"}}}},
   {"textRun":{"content":".\n","textStyle":{}}}]}},
 {"paragraph":{"paragraphStyle":{"namedStyleType":"NORMAL_TEXT"},"elements":[{"textRun":{"content":"\n"}}]}},
 {"paragraph":{"paragraphStyle":{"namedStyleType":"HEADING_2"},"elements":[{"textRun":{"content":"Steps\n"}}]}},
 {"paragraph":{"bullet":{"listId":"l1","nestingLevel":0},"elements":[{"textRun":{"content":"One\n"}}]}},
 {"paragraph":{"bullet":{"listId":"l1","nestingLevel":1},"elements":[{"textRun":{"content":"One point five\n"}}]}},
 {"paragraph":{"bullet":{"listId":"l1"},"elements":[{"textRun":{"content":"Two\n"}}]}},
 {"paragraph":{"paragraphStyle":{"namedStyleType":"NORMAL_TEXT"},"elements":[{"textRun":{"content":"line one\u000bline two\n"}}]}},
 {"table":{"tableRows":[
   {"tableCells":[{"content":[{"paragraph":{"elements":[{"textRun":{"content":"Name\n"}}]}}]},{"content":[{"paragraph":{"elements":[{"textRun":{"content":"Note\n"}}]}}]}]},
   {"tableCells":[{"content":[{"paragraph":{"elements":[{"textRun":{"content":"a|b\n"}}]}}]},{"content":[
       {"paragraph":{"elements":[{"textRun":{"content":"first\n"}}]}},{"paragraph":{"elements":[{"textRun":{"content":"second\n"}}]}}]}]}]}},
 {"paragraph":{"paragraphStyle":{"namedStyleType":"NORMAL_TEXT"},"elements":[{"textRun":{"content":"The end\n"}}]}}
]}}`

const docWant = "# Launch plan\n" +
	"\n" +
	"This is **very important** and *soft* see [the site](https://example.com/a).\n" +
	"\n" +
	"## Steps\n" +
	"\n" +
	"- One\n" +
	"  - One point five\n" +
	"- Two\n" +
	"\n" +
	"line one  \nline two\n" +
	"\n" +
	"| Name | Note |\n" +
	"| --- | --- |\n" +
	"| a\\|b | first second |\n" +
	"\n" +
	"The end"

func TestADocBecomesMarkdownWithHeadingsBulletsStylesLinksAndTables(t *testing.T) {
	c, g := newClient(t, always(200, docFixture))
	got, err := c.ReadDoc(context.Background(), "doc-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Launch plan" || got.Truncated {
		t.Errorf("content = %q truncated %v", got.Title, got.Truncated)
	}
	if got.Markdown != docWant {
		t.Errorf("markdown =\n%s\nwant\n%s", got.Markdown, docWant)
	}
	if req := g.all()[0]; req.method != http.MethodGet || req.path != "/v1/documents/doc-1" {
		t.Errorf("asked %s %s", req.method, req.path)
	}
}

func TestAStyleNeverWrapsTheSpacesBesideTheWords(t *testing.T) {
	got := renderSpans([]span{{text: " padded ", bold: true}, {text: "x", italic: true, link: "https://e.co"}}, true)
	if got != " **padded** *[x](https://e.co)*" {
		t.Errorf("rendered = %q", got)
	}
}

func TestAnErrorFromTheDocsAPIComesBackAsAnAPIError(t *testing.T) {
	c, _ := newClient(t, always(404, `{"error":{"code":404,"message":"Requested entity was not found.","status":"NOT_FOUND"}}`))
	_, err := c.ReadDoc(context.Background(), "gone")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !apiErr.NotFound() {
		t.Errorf("err = %v, want a not found", err)
	}
}

func TestASheetsFirstTabBecomesOneTableWithItsFirstRowAsTheHeader(t *testing.T) {
	c, g := newClient(t, func(r *http.Request, _ []byte) (int, string) {
		if strings.Contains(r.URL.Path, "/values/") {
			return 200, `{"range":"'Bob''s tab'!A1:Z200","majorDimension":"ROWS","values":[["Item","Cost"],["Tea | green","3"],["Long\nname"],[]]}`
		}
		return 200, `{"properties":{"title":"Budget"},"sheets":[{"properties":{"title":"Bob's tab"}},{"properties":{"title":"Other"}}]}`
	})
	got, err := c.ReadSheet(context.Background(), "sheet-1")
	if err != nil {
		t.Fatal(err)
	}
	want := "| Item | Cost |\n| --- | --- |\n| Tea \\| green | 3 |\n| Long name |  |\n|  |  |"
	if got.Title != "Budget" || got.Markdown != want || got.Truncated {
		t.Errorf("content = %q\n%s\ntruncated %v, want\n%s", got.Title, got.Markdown, got.Truncated, want)
	}
	reqs := g.all()
	if len(reqs) != 2 {
		t.Fatalf("%d calls, want 2", len(reqs))
	}
	if reqs[0].path != "/v4/spreadsheets/sheet-1" || reqs[0].query["fields"][0] != "properties.title,sheets.properties.title" {
		t.Errorf("first call = %s %v", reqs[0].path, reqs[0].query)
	}
	if reqs[1].path != "/v4/spreadsheets/sheet-1/values/'Bob''s tab'!A1:Z200" {
		t.Errorf("values call = %s, want the quoted tab and A1:Z200", reqs[1].path)
	}
}

func TestASheetThatFillsTheReadRangeIsMarkedAsCut(t *testing.T) {
	var rows []string
	for i := 0; i < sheetRows; i++ {
		rows = append(rows, `["x"]`)
	}
	c, _ := newClient(t, func(r *http.Request, _ []byte) (int, string) {
		if strings.Contains(r.URL.Path, "/values/") {
			return 200, `{"values":[` + strings.Join(rows, ",") + `]}`
		}
		return 200, `{"properties":{"title":"Big"},"sheets":[{"properties":{"title":"Sheet1"}}]}`
	})
	got, err := c.ReadSheet(context.Background(), "s")
	if err != nil || !got.Truncated {
		t.Errorf("a full range: %+v, %v, want truncated", got.Truncated, err)
	}
}

func TestASheetWithNoTabsOrNoCellsReadsEmpty(t *testing.T) {
	c, g := newClient(t, always(200, `{"properties":{"title":"Empty"},"sheets":[]}`))
	got, err := c.ReadSheet(context.Background(), "s")
	if err != nil || got.Title != "Empty" || got.Markdown != "" || len(g.all()) != 1 {
		t.Errorf("no tabs: %+v, %v after %d calls", got, err, len(g.all()))
	}
	c, _ = newClient(t, func(r *http.Request, _ []byte) (int, string) {
		if strings.Contains(r.URL.Path, "/values/") {
			return 200, `{"range":"Sheet1!A1:Z200","majorDimension":"ROWS"}`
		}
		return 200, `{"properties":{"title":"Blank"},"sheets":[{"properties":{"title":"Sheet1"}}]}`
	})
	if got, err := c.ReadSheet(context.Background(), "s"); err != nil || got.Markdown != "" {
		t.Errorf("no cells: %+v, %v", got, err)
	}
}

const slidesFixture = `{"title":"Roadmap","slides":[
 {"pageElements":[
   {"shape":{"placeholder":{"type":"CENTERED_TITLE"},"text":{"textElements":[{"paragraphMarker":{}},{"textRun":{"content":"Roadmap\n"}}]}}},
   {"shape":{"placeholder":{"type":"SUBTITLE"},"text":{"textElements":[{"textRun":{"content":"Q4 2026\n"}}]}}}]},
 {"pageElements":[
   {"shape":{"placeholder":{"type":"TITLE"},"text":{"textElements":[{"textRun":{"content":"Goals\n"}}]}}},
   {"shape":{"placeholder":{"type":"BODY"},"text":{"textElements":[{"textRun":{"content":"Ship it\n"}},{"textRun":{"content":"Tell people\n"}},{"textRun":{"content":"\n"}}]}}},
   {"elementGroup":{"children":[{"shape":{"text":{"textElements":[{"textRun":{"content":"In a group\n"}}]}}}]}},
   {"table":{"tableRows":[{"tableCells":[{"text":{"textElements":[{"textRun":{"content":"cell\n"}}]}}]}]}},
   {"line":{}}]},
 {"pageElements":[{"shape":{"text":{"textElements":[{"textRun":{"content":"Loose text\n"}}]}}}]}
]}`

func TestAPresentationBecomesAnOutlineWithATitlePerSlide(t *testing.T) {
	c, g := newClient(t, always(200, slidesFixture))
	got, err := c.ReadSlides(context.Background(), "deck-1")
	if err != nil {
		t.Fatal(err)
	}
	want := "## 1. Roadmap\n- Q4 2026\n\n## 2. Goals\n- Ship it\n- Tell people\n- In a group\n- cell\n\n## 3. Untitled\n- Loose text"
	if got.Title != "Roadmap" || got.Markdown != want {
		t.Errorf("outline =\n%s\nwant\n%s", got.Markdown, want)
	}
	if req := g.all()[0]; req.path != "/v1/presentations/deck-1" || req.rawQuery != "" {
		t.Errorf("asked %s ?%s, want the whole presentation", req.path, req.rawQuery)
	}
}

func TestLongMarkdownIsCutOnALineAndMarkedAsCut(t *testing.T) {
	line := strings.Repeat("a", 99) + "\n"
	long := strings.Repeat(line, MaxMarkdownBytes/len(line)+50)
	got, cut := capMarkdown(long)
	// Every line is 99 letters, so a cut on a line boundary leaves whole lines and no newline at the end.
	if !cut || len(got) > MaxMarkdownBytes || !strings.HasSuffix(got, "a") || (len(got)+1)%len(line) != 0 {
		t.Errorf("cut = %v, %d bytes", cut, len(got))
	}
	if got, cut := capMarkdown("short"); got != "short" || cut {
		t.Errorf("short text = %q, %v", got, cut)
	}
	oneLine := strings.Repeat("é", MaxMarkdownBytes)
	got, cut = capMarkdown(oneLine)
	if !cut || len(got) > MaxMarkdownBytes || strings.ContainsRune(got, '�') {
		t.Errorf("one huge line: cut %v, %d bytes, broken text %v", cut, len(got), strings.ContainsRune(got, '�'))
	}
}

func TestOnlyTheIdIsTakenFromAGoogleAddress(t *testing.T) {
	good := map[string]struct {
		kind Kind
		id   string
	}{
		"https://docs.google.com/document/d/1AbC_d-9/edit":                         {KindDoc, "1AbC_d-9"},
		"https://docs.google.com/document/d/1AbC_d-9/edit?usp=sharing#heading=h.x": {KindDoc, "1AbC_d-9"},
		"https://docs.google.com/document/u/0/d/1AbC_d-9/edit":                     {KindDoc, "1AbC_d-9"},
		"https://docs.google.com/spreadsheets/d/SHEET123/edit#gid=0":               {KindSheet, "SHEET123"},
		"https://docs.google.com/spreadsheets/u/2/d/SHEET123":                      {KindSheet, "SHEET123"},
		"https://docs.google.com/presentation/d/DECK-1/edit#slide=id.p":            {KindSlides, "DECK-1"},
		"  https://DOCS.google.com/document/d/ID1/  ":                              {KindDoc, "ID1"},
		"docs.google.com/document/d/ID1/edit":                                      {KindDoc, "ID1"},
		"http://docs.google.com/document/d/ID1/edit":                               {KindDoc, "ID1"},
	}
	for link, want := range good {
		kind, id, ok := ParseLink(link)
		if !ok || kind != want.kind || id != want.id {
			t.Errorf("ParseLink(%q) = %q %q %v, want %q %q", link, kind, id, ok, want.kind, want.id)
		}
	}
	for _, link := range []string{
		"", "hello", "https://example.com/document/d/ID1/edit", "https://docs.google.com.evil.com/document/d/ID1/edit",
		"https://docs.google.com@evil.com/document/d/ID1/edit", "https://evil.com/?u=https://docs.google.com/document/d/ID1",
		"https://docs.google.com:8443/document/d/ID1/edit", "https://docs.google.com/forms/d/ID1/edit",
		"https://docs.google.com/document/d/e/2PACX-published/pub", "https://docs.google.com/document/d/",
		"https://docs.google.com/document/",
		"https://docs.google.com/document/d/" + strings.Repeat("a", 201) + "/edit", "https://drive.google.com/file/d/ID1/view",
		"ftp://docs.google.com/document/d/ID1/edit", "https://docs.google.com/document/d/ID 1/edit",
		"javascript://docs.google.com/document/d/ID1/edit", "https://docs.google.com/document/u/0",
	} {
		if kind, id, ok := ParseLink(link); ok {
			t.Errorf("ParseLink(%q) = %q %q, want it refused", link, kind, id)
		}
	}
}
