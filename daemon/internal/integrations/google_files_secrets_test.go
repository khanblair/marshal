package integrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestACachedGrantIsNotSharedAfterAReconnect(t *testing.T) {
	// The folder is remembered per grant and name. A new grant asks Google again.
	f, g, files := newFilesFixture(t)
	f.connect(g, integrations.GDocsID)
	for i := 0; i < 2; i++ {
		if _, err := f.svc.CreateGoogleDoc(context.Background(), protocol.CreateGoogleDocRequest{Title: "T", Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	searches := func() int {
		n := 0
		for _, req := range files.Requests() {
			if req.Method == http.MethodGet && req.Path == "/drive/v3/files" && strings.Contains(req.Query.Get("q"), "folder") {
				n++
			}
		}
		return n
	}
	if searches() != 1 {
		t.Fatalf("%d folder searches for two saves, want 1", searches())
	}
	f.connect(g, integrations.GDocsID)
	if _, err := f.svc.CreateGoogleDoc(context.Background(), protocol.CreateGoogleDocRequest{Title: "T", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if searches() != 2 {
		t.Errorf("%d folder searches after a reconnect, want 2", searches())
	}
}

func TestGmailAndCalendarKeepTheirOwnSecretsBesideTheFileConnections(t *testing.T) {
	f, g, _ := newFilesFixture(t)
	f.connect(g, integrations.GCalID)
	f.connect(g, integrations.GmailID)
	f.connect(g, integrations.GDocsID)
	raw, err := f.keys.Get(integrations.GCalID)
	if err != nil {
		t.Fatal(err)
	}
	var calendar struct {
		ClientSecret string        `json:"clientSecret"`
		Token        *oauth2.Token `json:"token"`
	}
	if err := json.Unmarshal([]byte(raw), &calendar); err != nil || calendar.ClientSecret != "client-secret" || calendar.Token == nil {
		t.Errorf("calendar secret = %q, want its client secret and token", raw)
	}
	if _, err := f.svc.GmailClient(context.Background()); err != nil {
		t.Errorf("Gmail: %v", err)
	}
	if _, err := f.svc.GoogleCalendarClient(context.Background()); err != nil {
		t.Errorf("Calendar: %v", err)
	}
}

func TestAGoogleThatCannotBeReachedIsAPlainSentenceAndNotAnInternalError(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	f, g, _ := newFilesFixture(t, func(o *integrations.Options) { o.GoogleFilesBaseURL = dead.URL })
	f.connect(g, integrations.GDocsID)
	ctx := context.Background()
	want := "Marshal could not reach Google. Check this computer's connection, then try again."
	_, err := f.svc.CreateGoogleDoc(ctx, protocol.CreateGoogleDocRequest{Title: "T", Text: "x"})
	var refusal *protocol.Error
	if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeUnavailable || refusal.Message != want {
		t.Errorf("making a doc: err = %v", err)
	}
	_, err = f.svc.GoogleFiles(ctx, protocol.GoogleFileKindDoc)
	if !errors.As(err, &refusal) || refusal.Message != want {
		t.Errorf("listing: err = %v", err)
	}
	_, err = f.svc.ReadGoogleLink(ctx, protocol.ReadGoogleLinkRequest{URL: "https://docs.google.com/document/d/DOC1/edit"})
	if !errors.As(err, &refusal) || refusal.Message != want {
		t.Errorf("reading: err = %v", err)
	}
}
