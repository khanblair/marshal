// Package gmailread is Marshal's Gmail client: the one call that reads a label's messages
// (B8.3, docs/marshal-product-scope.md 18-19). Read-only: the scope Marshal asks for cannot mark
// a message read, relabel it, or send anything (hard rule 2).
package gmailread

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Scope is what Marshal asks Gmail for.
const Scope = gmail.GmailReadonlyScope

// Client wraps the generated Gmail service.
type Client struct {
	svc *gmail.Service
}

// New builds a Client. httpClient is normally an OAuth2 config's own client for a live token; a
// test passes one that never leaves the process. baseURL overrides where the API is reached, for
// a test; empty means the real Google API.
func New(ctx context.Context, httpClient *http.Client, baseURL string) (*Client, error) {
	svc, err := gmail.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("build the Gmail client: %w", err)
	}
	if baseURL != "" {
		svc.BasePath = baseURL
	}
	return &Client{svc: svc}, nil
}

// Message is one labeled email Marshal reads back.
type Message struct {
	ID      string
	Subject string
	From    string
	Snippet string
}

// Labeled reads the newest messages carrying label, newest-affecting-search-order first (Gmail's
// own relevance order, since the API has no explicit newest-first sort). max bounds how many are
// read; Gmail's search itself limits the id list, so this is a second, defensive cap.
func (c *Client) Labeled(ctx context.Context, label string, max int) ([]Message, error) {
	list, err := c.svc.Users.Messages.List("me").Q("label:" + label).MaxResults(int64(max)).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("search Gmail for label:%s: %w", label, err)
	}
	out := make([]Message, 0, len(list.Messages))
	for _, item := range list.Messages {
		full, err := c.svc.Users.Messages.Get("me", item.Id).Format("metadata").Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("read the Gmail message %s: %w", item.Id, err)
		}
		out = append(out, messageOf(full))
	}
	return out, nil
}

// messageOf reads a message's Subject and From headers and its snippet. A header that is not
// there is left empty rather than failing the whole read.
func messageOf(msg *gmail.Message) Message {
	m := Message{ID: msg.Id, Snippet: msg.Snippet}
	if msg.Payload == nil {
		return m
	}
	for _, header := range msg.Payload.Headers {
		switch header.Name {
		case "Subject":
			m.Subject = header.Value
		case "From":
			m.From = header.Value
		}
	}
	return m
}

// About answers how many messages this token can read with no query, for the connection test: the
// cheapest call that proves the token actually works for Gmail specifically (Calendar's own token
// can be valid while Gmail's scope was never granted, since a person can grant Calendar alone).
func (c *Client) About(ctx context.Context) (messages int64, err error) {
	profile, err := c.svc.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("read the Gmail profile this token can reach: %w", err)
	}
	return profile.MessagesTotal, nil
}
