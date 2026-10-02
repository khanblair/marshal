package gmailread

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fake(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/gmail/v1/users/me/messages/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/messages/")
		_, _ = w.Write([]byte(`{"id":"` + id + `","snippet":"snip ` + id + `","payload":{"headers":[
			{"name":"Subject","value":"Subject ` + id + `"},{"name":"From","value":"a@b.co"},{"name":"X-Other","value":"ignored"}]}}`))
	})
	mux.HandleFunc("/gmail/v1/users/me/messages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.Contains(r.URL.Query().Get("q"), "label:marshal") {
			http.Error(w, `{"error":{"code":400}}`, http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"messages":[{"id":"m1"},{"id":"m2"}]}`))
	})
	mux.HandleFunc("/gmail/v1/users/me/profile", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"emailAddress":"me@x.com","messagesTotal":42}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	client, err := New(context.Background(), server.Client(), server.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestScopeIsReadOnly(t *testing.T) {
	if !strings.HasSuffix(Scope, "gmail.readonly") {
		t.Errorf("scope = %q, want read-only", Scope)
	}
}

func TestLabeledReadsSubjectFromAndSnippetOfEachMessage(t *testing.T) {
	got, err := fake(t).Labeled(context.Background(), "marshal", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "m1" || got[0].Subject != "Subject m1" || got[0].From != "a@b.co" || got[1].Snippet != "snip m2" {
		t.Errorf("messages = %+v", got)
	}
}

func TestAboutCountsTheMailboxToProveTheTokenWorks(t *testing.T) {
	total, err := fake(t).About(context.Background())
	if err != nil || total != 42 {
		t.Errorf("About = %d, %v, want 42", total, err)
	}
}
