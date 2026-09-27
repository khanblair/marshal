package github_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/github"
)

// actionsServer starts a fake GitHub whose handler sees the raw request (so a query string is
// recorded too) and returns a client pointed at it.
func actionsServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*github.TokenClient, *[]string) {
	t.Helper()
	seen := &[]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.Method+" "+r.URL.RequestURI())
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	client, err := github.NewTokenClient("ghp_test_token", github.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewTokenClient: %v", err)
	}
	return client, seen
}

// zipLog makes the archive GitHub answers a job-log request with: one file per step.
func zipLog(t *testing.T, files ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for i, contents := range files {
		entry, err := writer.Create("step" + string(rune('0'+i)) + ".txt")
		if err != nil {
			t.Fatalf("build the log archive: %v", err)
		}
		if _, err := entry.Write([]byte(contents)); err != nil {
			t.Fatalf("build the log archive: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("build the log archive: %v", err)
	}
	return buf.Bytes()
}

func TestListWorkflowRunsAsksForTheBranchAndReadsTheRuns(t *testing.T) {
	client, seen := actionsServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"workflow_runs": []map[string]any{{
				"id": 9931, "name": "ci", "head_branch": "marshal/card-12",
				"status": "completed", "conclusion": "failure",
				"html_url":       "https://github.com/acme/web/actions/runs/9931",
				"run_started_at": "2026-09-27T09:26:00Z",
			}},
		})
	})

	runs, err := client.ListWorkflowRuns(context.Background(), github.Repository{Owner: "acme", Name: "web"}, "marshal/card-12")
	if err != nil {
		t.Fatalf("ListWorkflowRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != 9931 || runs[0].Name != "ci" || runs[0].Conclusion != "failure" {
		t.Fatalf("runs = %+v", runs)
	}
	if !runs[0].Failed() {
		t.Error("a completed failure is not read as failed")
	}
	if len(*seen) != 1 || !strings.Contains((*seen)[0], "branch=marshal%2Fcard-12") {
		t.Errorf("requests = %v, want the branch asked for", *seen)
	}
}

// A run that is still going, and one that was skipped, are not failures to fix.
func TestOnlyACompletedFailureIsAFailure(t *testing.T) {
	for _, run := range []github.WorkflowRun{
		{Status: "in_progress", Conclusion: ""},
		{Status: "queued"},
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "skipped"},
		{Status: "completed", Conclusion: "cancelled"},
	} {
		if run.Failed() {
			t.Errorf("%+v was read as failed", run)
		}
	}
	if !(github.WorkflowRun{Status: "completed", Conclusion: "timed_out"}).Failed() {
		t.Error("a timed-out run is a failure to fix")
	}
}

func TestRerunFailedJobsPostsToTheRun(t *testing.T) {
	var gotMethod, gotPath string
	client, _ := actionsServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusCreated)
	})

	if err := client.RerunFailedJobs(context.Background(), github.Repository{Owner: "acme", Name: "web"}, 9931); err != nil {
		t.Fatalf("RerunFailedJobs: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/repos/acme/web/actions/runs/9931/rerun-failed-jobs" {
		t.Errorf("path = %s", gotPath)
	}
}

// The log is read from the failed job only: the jobs that passed printed nothing the card needs.
func TestFailedLogReadsOnlyTheFailedJob(t *testing.T) {
	client, seen := actionsServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/runs/9931/jobs"):
			writeJSON(t, w, http.StatusOK, map[string]any{
				"jobs": []map[string]any{
					{"id": 11, "name": "lint", "conclusion": "success"},
					{"id": 12, "name": "test", "conclusion": "failure", "steps": []map[string]any{
						{"name": "Install", "conclusion": "success"},
						{"name": "Run tests", "conclusion": "failure"},
					}},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/jobs/12/logs"):
			_, _ = w.Write([]byte("FAIL src/report.test.ts\n  expected 3, got 2\n"))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	log, err := client.FailedLog(context.Background(), github.Repository{Owner: "acme", Name: "web"}, 9931, 0)
	if err != nil {
		t.Fatalf("FailedLog: %v", err)
	}
	if !strings.HasPrefix(log, "# test / Run tests\n") {
		t.Errorf("log does not name the job and its failed step: %q", log)
	}
	if !strings.Contains(log, "expected 3, got 2") {
		t.Errorf("log = %q", log)
	}
	var askedLog bool
	for _, req := range *seen {
		if strings.Contains(req, "/jobs/11/logs") {
			t.Error("the passing job's log was read")
		}
		if strings.Contains(req, "/jobs/12/logs") {
			askedLog = true
		}
	}
	if !askedLog {
		t.Errorf("requests = %v, want the failed job's log", *seen)
	}
}

// GitHub answers a job's log address with a zip of one file per step. It is unzipped, and the files
// are joined in order.
func TestFailedLogUnzipsAnArchive(t *testing.T) {
	archive := zipLog(t, "step one\n", "step two\n")
	client, _ := actionsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/jobs") || strings.HasSuffix(r.URL.Path, "/runs/9931/jobs") {
			writeJSON(t, w, http.StatusOK, map[string]any{
				"jobs": []map[string]any{{"id": 12, "name": "test", "conclusion": "failure"}},
			})
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive)
	})

	log, err := client.FailedLog(context.Background(), github.Repository{Owner: "acme", Name: "web"}, 9931, 0)
	if err != nil {
		t.Fatalf("FailedLog: %v", err)
	}
	if !strings.Contains(log, "step one") || !strings.Contains(log, "step two") {
		t.Errorf("log = %q", log)
	}
	if strings.Index(log, "step one") > strings.Index(log, "step two") {
		t.Errorf("the archive's files were joined out of order: %q", log)
	}
}

// A run with no failed job answers an empty log rather than an error: a cancelled run is reported
// failed with nothing to read.
func TestFailedLogIsEmptyWhenNoJobFailed(t *testing.T) {
	client, _ := actionsServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"jobs": []map[string]any{{"id": 11, "name": "lint", "conclusion": "cancelled"}},
		})
	})
	log, err := client.FailedLog(context.Background(), github.Repository{Owner: "acme", Name: "web"}, 9931, 0)
	if err != nil {
		t.Fatalf("FailedLog: %v", err)
	}
	if log != "" {
		t.Errorf("log = %q, want empty", log)
	}
}

// A forge failure is an *APIError, so a caller switches on the status the same way every other
// call in this package does.
func TestAFailedActionsCallIsAnAPIError(t *testing.T) {
	client, _ := actionsServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusForbidden, map[string]any{"message": "Resource not accessible"})
	})
	err := client.RerunFailedJobs(context.Background(), github.Repository{Owner: "acme", Name: "web"}, 9931)
	var api *github.APIError
	if !errors.As(err, &api) || api.Status != http.StatusForbidden {
		t.Fatalf("err = %v, want an APIError with a 403", err)
	}
}

// The log's size limit is honoured: a step that prints forever is cut, not read forever.
func TestFailedLogIsCutToTheLimit(t *testing.T) {
	client, _ := actionsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/jobs") {
			writeJSON(t, w, http.StatusOK, map[string]any{
				"jobs": []map[string]any{{"id": 12, "name": "test", "conclusion": "failure"}},
			})
			return
		}
		_, _ = w.Write(bytes.Repeat([]byte("x"), 4096))
	})
	log, err := client.FailedLog(context.Background(), github.Repository{Owner: "acme", Name: "web"}, 9931, 64)
	if err != nil {
		t.Fatalf("FailedLog: %v", err)
	}
	if len(log) > 128 {
		t.Errorf("log is %d bytes, want it cut near the 64-byte limit", len(log))
	}
}
