package googlefiles

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// keyObjectID is the name Slides gives the id of a slide or a shape in a request.
const keyObjectID = "objectId"

// Slide is one slide Marshal makes: a title and the lines under it.
type Slide struct {
	Title   string
	Bullets []string
}

// CreateSlides makes a Google Slides presentation. Drive makes it with one blank slide, so the
// slides are added in one batch of changes, and the blank one is removed in the same batch.
func (c *Client) CreateSlides(ctx context.Context, folderID, title string, slides []Slide) (File, error) {
	meta := metaIn(folderID, title, MimeSlides)
	var made driveFile
	if err := c.postJSON(ctx, APIDrive, "/drive/v3/files", url.Values{fieldsParam: {fileFields}}, meta, &made); err != nil {
		return File{}, fmt.Errorf("make the presentation %q in Drive: %w", title, err)
	}
	file, err := checkedFile(made)
	if err != nil {
		return File{}, err
	}
	path := "/v1/presentations/" + url.PathEscape(file.ID)
	var blank struct {
		Slides []struct {
			ObjectID string `json:"objectId"`
		} `json:"slides"`
	}
	if err := c.getJSON(ctx, APISlides, path, url.Values{fieldsParam: {"slides.objectId"}}, &blank); err != nil {
		return File{}, fmt.Errorf("read the new presentation %s: %w", file.ID, err)
	}
	var first string
	if len(blank.Slides) > 0 {
		first = blank.Slides[0].ObjectID
	}
	body := map[string]any{"requests": slideRequests(slides, first)}
	if err := c.postJSON(ctx, APISlides, path+":batchUpdate", nil, body, nil); err != nil {
		return File{}, fmt.Errorf("fill the new presentation %s: %w", file.ID, err)
	}
	return file, nil
}

// slideRequests are the changes that make the slides: for each, a new slide with a title and a body,
// then the text of each, and last the removal of the blank slide Drive made.
func slideRequests(slides []Slide, blank string) []map[string]any {
	var requests []map[string]any
	for i, slide := range slides {
		n := i + 1
		slideID, titleID, bodyID := fmt.Sprintf("m_slide_%d", n), fmt.Sprintf("m_title_%d", n), fmt.Sprintf("m_body_%d", n)
		requests = append(requests, map[string]any{"createSlide": map[string]any{
			keyObjectID:            slideID,
			"slideLayoutReference": map[string]any{"predefinedLayout": "TITLE_AND_BODY"},
			"placeholderIdMappings": []map[string]any{
				{"layoutPlaceholder": map[string]any{"type": "TITLE", "index": 0}, keyObjectID: titleID},
				{"layoutPlaceholder": map[string]any{"type": "BODY", "index": 0}, keyObjectID: bodyID},
			},
		}})
		// Slides refuses an insert of no text, so an empty title or body is left alone.
		if slide.Title != "" {
			requests = append(requests, insertText(titleID, slide.Title))
		}
		if body := strings.Join(slide.Bullets, "\n"); body != "" {
			requests = append(requests, insertText(bodyID, body))
		}
	}
	if blank != "" {
		requests = append(requests, map[string]any{"deleteObject": map[string]any{keyObjectID: blank}})
	}
	return requests
}

func insertText(objectID, text string) map[string]any {
	return map[string]any{"insertText": map[string]any{keyObjectID: objectID, "text": text}}
}
