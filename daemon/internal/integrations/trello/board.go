package trello

// The calls the Trello sync needs beyond the first ones: reading a whole board with its checklists
// and comments, and the writes the person's Apply sends (docs/marshal-product-scope.md section 19.2).
// Every call goes through call, which keeps the key and the token out of any error it returns.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// commentPage is how many comments one actions call asks for, which is Trello's own largest page.
const commentPage = 1000

// maxCommentPages stops a board with a very long history from being read forever. Five thousand
// comments is far more than a board Marshal links is expected to hold.
const maxCommentPages = 5

// Checklist is one checklist on a card.
type Checklist struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Items []CheckItem `json:"checkItems"`
}

// CheckItem is one line of a checklist. State is "complete" or "incomplete".
type CheckItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// Done reports whether the line is ticked.
func (i CheckItem) Done() bool { return i.State == "complete" }

// Comment is one comment on a card, read from the board's activity.
type Comment struct {
	// ID is the action's id, which is the comment's id.
	ID string
	// CardID is the card the comment is on.
	CardID string
	// Text is what it says.
	Text string
	// Date is when it was posted, as Trello writes it (RFC 3339).
	Date string
	// Author is the commenter's full name, or their username when the name is empty.
	Author string
}

// commentAction is how Trello writes a comment inside an action.
type commentAction struct {
	ID   string `json:"id"`
	Date string `json:"date"`
	Data struct {
		Text string `json:"text"`
		Card struct {
			ID string `json:"id"`
		} `json:"card"`
	} `json:"data"`
	MemberCreator struct {
		FullName string `json:"fullName"`
		Username string `json:"username"`
	} `json:"memberCreator"`
}

// call sends one request and decodes the JSON answer into out when out is not nil. The key and the
// token are added here, so a caller never holds a URL that contains them, and an error from the
// network layer, which Go writes with the whole URL, is cut down to its cause.
func (c *Client) call(ctx context.Context, method, path string, query url.Values, out any) error {
	auth := url.Values{}
	auth.Set("key", c.apiKey)
	auth.Set("token", c.token)
	target := c.baseURL + path + "?" + auth.Encode()
	var body io.Reader
	if method == http.MethodGet {
		// A read carries its options in the address, with the credential.
		merged := url.Values{}
		for k, vs := range query {
			merged[k] = vs
		}
		merged.Set("key", c.apiKey)
		merged.Set("token", c.token)
		target = c.baseURL + path + "?" + merged.Encode()
	} else {
		// A write carries its fields in the body, so a long comment or description is not limited by
		// the length of an address.
		body = strings.NewReader(query.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return fmt.Errorf("build the Trello request for %s: %w", path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("reach Trello for %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return &APIError{Status: res.StatusCode, Path: path, Body: strings.TrimSpace(string(body))}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("read Trello's answer for %s: %w", path, err)
	}
	return nil
}

// APIError is an answer from Trello that was not a success.
type APIError struct {
	// Status is the HTTP status.
	Status int
	// Path is the call that was refused.
	Path string
	// Body is the start of what Trello said.
	Body string
}

// Error says what Trello answered.
func (e *APIError) Error() string {
	return fmt.Sprintf("trello API error %d for %s: %s", e.Status, e.Path, e.Body)
}

// NotFound reports whether Trello said the thing is gone.
func (e *APIError) NotFound() bool { return e.Status == http.StatusNotFound }

// IsNotFound reports whether err is Trello saying a card, a list, or a checklist no longer exists.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.NotFound()
}

// BoardCards reads every open card on a board with its checklists and items. One call, however many
// cards: Trello answers up to a thousand.
func (c *Client) BoardCards(ctx context.Context, boardID string) ([]Card, error) {
	var cards []Card
	q := url.Values{}
	q.Set("fields", "name,desc,idList,closed,url,due,dateLastActivity")
	q.Set("checklists", "all")
	q.Set("checklist_fields", "name")
	err := c.call(ctx, http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/cards/open", q, &cards)
	if err != nil {
		return nil, err
	}
	return cards, nil
}

// GetCard reads one card with its checklists, which is what Apply compares against before it writes.
func (c *Client) GetCard(ctx context.Context, cardID string) (Card, error) {
	var card Card
	q := url.Values{}
	q.Set("fields", "name,desc,idList,closed,url,due,dateLastActivity")
	q.Set("checklists", "all")
	q.Set("checklist_fields", "name")
	if err := c.call(ctx, http.MethodGet, "/cards/"+url.PathEscape(cardID), q, &card); err != nil {
		return Card{}, err
	}
	return card, nil
}

// BoardComments reads the comments on a board's cards, newest first, a page at a time up to a limit.
func (c *Client) BoardComments(ctx context.Context, boardID string) ([]Comment, error) {
	var out []Comment
	before := ""
	for page := 0; page < maxCommentPages; page++ {
		q := url.Values{}
		q.Set("filter", "commentCard")
		q.Set("limit", fmt.Sprint(commentPage))
		if before != "" {
			q.Set("before", before)
		}
		var actions []commentAction
		if err := c.call(ctx, http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/actions", q, &actions); err != nil {
			return nil, err
		}
		for _, a := range actions {
			author := a.MemberCreator.FullName
			if author == "" {
				author = a.MemberCreator.Username
			}
			out = append(out, Comment{
				ID: a.ID, CardID: a.Data.Card.ID, Text: a.Data.Text, Date: a.Date, Author: author,
			})
		}
		if len(actions) < commentPage {
			break
		}
		before = actions[len(actions)-1].ID
	}
	return out, nil
}

// UpdateCardFields are the fields of a card Marshal may change. A nil field is left as it is.
type UpdateCardFields struct {
	Name *string
	Desc *string
}

// UpdateCard writes a new name or description.
func (c *Client) UpdateCard(ctx context.Context, cardID string, fields UpdateCardFields) error {
	q := url.Values{}
	if fields.Name != nil {
		q.Set("name", *fields.Name)
	}
	if fields.Desc != nil {
		q.Set("desc", *fields.Desc)
	}
	return c.call(ctx, http.MethodPut, "/cards/"+url.PathEscape(cardID), q, nil)
}

// AddComment posts a comment as the person the token belongs to. It answers the new comment's id.
func (c *Client) AddComment(ctx context.Context, cardID, text string) (string, error) {
	var action commentAction
	q := url.Values{}
	q.Set("text", text)
	if err := c.call(ctx, http.MethodPost, "/cards/"+url.PathEscape(cardID)+"/actions/comments", q, &action); err != nil {
		return "", err
	}
	return action.ID, nil
}

// CreateChecklist adds an empty checklist to a card.
func (c *Client) CreateChecklist(ctx context.Context, cardID, name string) (Checklist, error) {
	var list Checklist
	q := url.Values{}
	q.Set("idCard", cardID)
	q.Set("name", name)
	if err := c.call(ctx, http.MethodPost, "/checklists", q, &list); err != nil {
		return Checklist{}, err
	}
	return list, nil
}

// AddCheckItem adds one line to a checklist.
func (c *Client) AddCheckItem(ctx context.Context, checklistID, name string, done bool) (CheckItem, error) {
	var item CheckItem
	q := url.Values{}
	q.Set("name", name)
	q.Set("checked", fmt.Sprint(done))
	if err := c.call(ctx, http.MethodPost, "/checklists/"+url.PathEscape(checklistID)+"/checkItems", q, &item); err != nil {
		return CheckItem{}, err
	}
	return item, nil
}

// SetCheckItem ticks or reopens one line of a card's checklist.
func (c *Client) SetCheckItem(ctx context.Context, cardID, itemID string, done bool) error {
	state := "incomplete"
	if done {
		state = "complete"
	}
	q := url.Values{}
	q.Set("state", state)
	return c.call(ctx, http.MethodPut, "/cards/"+url.PathEscape(cardID)+"/checkItem/"+url.PathEscape(itemID), q, nil)
}
