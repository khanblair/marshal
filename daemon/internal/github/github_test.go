package github_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/github"
)

// newServer starts a fake GitHub and returns a client pointed at it, plus the recorded requests.
// Every request is recorded so a test can prove the path, the method, and the headers.
type recorded struct {
	method string
	path   string
	body   map[string]any
	auth   string
	accept string
}

func newServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body map[string]any)) (*github.TokenClient, *[]recorded) {
	t.Helper()
	seen := &[]recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			raw, _ := io.ReadAll(r.Body)
			if len(raw) > 0 {
				_ = json.Unmarshal(raw, &body)
			}
		}
		*seen = append(*seen, recorded{
			method: r.Method, path: r.URL.EscapedPath(), body: body,
			auth: r.Header.Get("Authorization"), accept: r.Header.Get("Accept"),
		})
		handler(w, r, body)
	}))
	t.Cleanup(srv.Close)
	client, err := github.NewTokenClient("ghp_test_token", github.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewTokenClient: %v", err)
	}
	return client, seen
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode answer: %v", err)
	}
}

func TestCreatePullRequestSendsTheCardBranchAndReadsTheAnswer(t *testing.T) {
	client, seen := newServer(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
		writeJSON(t, w, http.StatusCreated, map[string]any{
			"number":   287,
			"html_url": "https://github.com/acme/web/pull/287",
			"state":    "open",
			"title":    "Add retry",
			"head":     map[string]any{"ref": "marshal/card-41"},
			"base":     map[string]any{"ref": "main"},
		})
	})

	pr, err := client.CreatePullRequest(context.Background(), github.NewPullRequest{
		Repo:  github.Repository{Owner: "acme", Name: "web"},
		Title: "Add retry",
		Body:  "Closes #41",
		Head:  "marshal/card-41",
		Base:  "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if pr.Number != 287 || pr.URL != "https://github.com/acme/web/pull/287" {
		t.Errorf("pull request = %+v, want #287 at its address", pr)
	}
	if pr.Head != "marshal/card-41" || pr.Base != "main" {
		t.Errorf("head/base = %q/%q, want the card's branch into main", pr.Head, pr.Base)
	}
	got := (*seen)[0]
	if got.method != http.MethodPost || got.path != "/repos/acme/web/pulls" {
		t.Errorf("request = %s %s, want POST /repos/acme/web/pulls", got.method, got.path)
	}
	if got.auth != "Bearer ghp_test_token" {
		t.Errorf("Authorization = %q, want the personal token", got.auth)
	}
	if got.accept != github.AcceptHeader {
		t.Errorf("Accept = %q, want %q", got.accept, github.AcceptHeader)
	}
	if got.body["head"] != "marshal/card-41" || got.body["base"] != "main" || got.body["title"] != "Add retry" {
		t.Errorf("body = %+v, want the head, base, and title", got.body)
	}
	if got.body["draft"] != false {
		t.Errorf("draft = %v, want false", got.body["draft"])
	}
}

func TestGetPullRequestReadsOne(t *testing.T) {
	client, seen := newServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"number": 12, "html_url": "https://example.test/pull/12", "state": "closed",
			"head": map[string]any{"ref": "work"}, "base": map[string]any{"ref": "main"},
		})
	})
	pr, err := client.GetPullRequest(context.Background(), github.Repository{Owner: "acme", Name: "web"}, 12)
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	if pr.Number != 12 || pr.State != "closed" {
		t.Errorf("pull request = %+v, want #12 closed", pr)
	}
	if (*seen)[0].path != "/repos/acme/web/pulls/12" {
		t.Errorf("path = %q, want the pull request's own path", (*seen)[0].path)
	}
}

func TestCreateReviewCommentOnThePullRequestAndOnALine(t *testing.T) {
	client, seen := newServer(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
		if strings.Contains(r.URL.Path, "/issues/") {
			writeJSON(t, w, http.StatusCreated, map[string]any{"id": 1, "html_url": "https://example.test/c/1", "body": "looks good"})
			return
		}
		writeJSON(t, w, http.StatusCreated, map[string]any{"id": 2, "path": "main.go", "line": 9, "body": "off by one"})
	})

	if _, err := client.CreateReviewComment(context.Background(), github.NewReviewComment{
		Repo: github.Repository{Owner: "acme", Name: "web"}, Number: 7, Body: "looks good",
	}); err != nil {
		t.Fatalf("summary comment: %v", err)
	}
	if (*seen)[0].path != "/repos/acme/web/issues/7/comments" {
		t.Errorf("summary path = %q, want the issue-comment endpoint", (*seen)[0].path)
	}

	line, err := client.CreateReviewComment(context.Background(), github.NewReviewComment{
		Repo: github.Repository{Owner: "acme", Name: "web"}, Number: 7,
		Body: "off by one", Path: "main.go", Line: 9,
	})
	if err != nil {
		t.Fatalf("line comment: %v", err)
	}
	if line.Line != 9 {
		t.Errorf("line = %d, want 9", line.Line)
	}
	got := (*seen)[1]
	if got.path != "/repos/acme/web/pulls/7/comments" {
		t.Errorf("line path = %q, want the review-comment endpoint", got.path)
	}
	if got.body["side"] != "RIGHT" || got.body["path"] != "main.go" || got.body["line"] != float64(9) {
		t.Errorf("line body = %+v, want path, line, and RIGHT side", got.body)
	}
}

func TestListChecksReadsTheCheckRuns(t *testing.T) {
	client, seen := newServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"check_runs": []map[string]any{
				{"name": "test", "status": "completed", "conclusion": "success"},
				{"name": "lint", "status": "in_progress", "conclusion": ""},
			},
		})
	})
	checks, err := client.ListChecks(context.Background(), github.Repository{Owner: "acme", Name: "web"}, "marshal/card-41")
	if err != nil {
		t.Fatalf("ListChecks: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("checks = %d, want 2", len(checks))
	}
	if !checks[0].Passed() {
		t.Errorf("test check should have passed: %+v", checks[0])
	}
	if checks[1].Passed() {
		t.Errorf("an in-progress check must not read as passed: %+v", checks[1])
	}
	if (*seen)[0].path != "/repos/acme/web/commits/marshal%2Fcard-41/check-runs" {
		t.Errorf("path = %q, want the ref escaped in the check-runs path", (*seen)[0].path)
	}
}

func TestListChecksWithNoRunsIsAnEmptyList(t *testing.T) {
	client, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		writeJSON(t, w, http.StatusOK, map[string]any{})
	})
	checks, err := client.ListChecks(context.Background(), github.Repository{Owner: "a", Name: "b"}, "main")
	if err != nil {
		t.Fatalf("ListChecks: %v", err)
	}
	if checks == nil || len(checks) != 0 {
		t.Errorf("checks = %#v, want an empty non-nil list", checks)
	}
}

func TestFailedCallsBecomeAPIErrors(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		wantIs     error
		wantStatus int
	}{
		{"unauthorized", http.StatusUnauthorized, github.ErrUnauthorized, 401},
		{"not found", http.StatusNotFound, github.ErrNotFound, 404},
		{"refused", http.StatusUnprocessableEntity, nil, 422},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
				writeJSON(t, w, tc.status, map[string]any{"message": "nope", "documentation_url": "https://docs.test"})
			})
			_, err := client.GetPullRequest(context.Background(), github.Repository{Owner: "a", Name: "b"}, 1)
			if err == nil {
				t.Fatal("want an error")
			}
			if github.Status(err) != tc.wantStatus {
				t.Errorf("Status = %d, want %d", github.Status(err), tc.wantStatus)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("errors.Is(%v, %v) = false, want true", err, tc.wantIs)
			}
			if !strings.Contains(err.Error(), "nope") {
				t.Errorf("error = %q, want the forge's message", err)
			}
		})
	}
}

func TestErrorsNeverRepeatTheToken(t *testing.T) {
	client, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "Bad credentials")
	})
	_, err := client.ListChecks(context.Background(), github.Repository{Owner: "a", Name: "b"}, "main")
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "ghp_test_token") {
		t.Errorf("error repeats the token: %q", err)
	}
}

func TestValidationRefusesWhatWouldBeAnEmptyCall(t *testing.T) {
	client, seen := newServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		writeJSON(t, w, http.StatusOK, map[string]any{})
	})
	cases := []struct {
		name string
		call func() error
	}{
		{"no repository", func() error {
			_, err := client.GetPullRequest(context.Background(), github.Repository{}, 1)
			return err
		}},
		{"no title", func() error {
			_, err := client.CreatePullRequest(context.Background(), github.NewPullRequest{
				Repo: github.Repository{Owner: "a", Name: "b"}, Head: "h", Base: "m",
			})
			return err
		}},
		{"no head", func() error {
			_, err := client.CreatePullRequest(context.Background(), github.NewPullRequest{
				Repo: github.Repository{Owner: "a", Name: "b"}, Title: "t", Base: "m",
			})
			return err
		}},
		{"no comment body", func() error {
			_, err := client.CreateReviewComment(context.Background(), github.NewReviewComment{
				Repo: github.Repository{Owner: "a", Name: "b"}, Number: 1,
			})
			return err
		}},
		{"no ref", func() error {
			_, err := client.ListChecks(context.Background(), github.Repository{Owner: "a", Name: "b"}, "")
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Errorf("%s: want a refusal", tc.name)
			}
		})
	}
	if len(*seen) != 0 {
		t.Errorf("a refused call still reached the server: %+v", *seen)
	}
}

func TestNewTokenClientNeedsAToken(t *testing.T) {
	if _, err := github.NewTokenClient("   "); err == nil {
		t.Fatal("want an error for an empty token")
	}
	if _, err := github.NewTokenClient("ghp_x"); err != nil {
		t.Fatalf("NewTokenClient with a token: %v", err)
	}
}
