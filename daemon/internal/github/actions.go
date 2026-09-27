package github

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// This file is the Actions half of the forge client: the three calls the CI monitor makes when a
// workflow on a card's branch fails (docs/architecture.md section 9, docs/backend-checklist.md
// B6.2 and B6.3, build-plan 6.2 and 6.3). Like the rest of the package it is the only place
// Marshal speaks GitHub's HTTP, and it is written once on `caller`, so Phase 5's personal-token
// client and Phase 6's App client both have it.
//
// The three calls are exactly section 9's three steps and no more: ask the forge to run a run's
// failed jobs again, read the failed job's own log, and ask what runs exist on a branch (the
// polling backup that covers a delivery GitHub did not send). Nothing here decides anything; the
// CI monitor owns the rules.

// maxLogBytes bounds how much of a job's log is read from the forge. A step that prints forever
// must not fill memory before the monitor has a chance to trim it; the monitor trims further.
const maxLogBytes = 512 << 10

// WorkflowRun is one Actions run of one workflow on one branch, as the forge reports it. It is the
// forge's own answer, not a wire type: the CI monitor turns it into a protocol.CiRun.
type WorkflowRun struct {
	// ID is the forge's numeric run id. It is what a rerun is asked for by, and it is stable across
	// a rerun of the same run, which is what lets the monitor tell a first failure from a second.
	ID int64 `json:"id"`
	// Name is the workflow's name as the forge words it, such as "ci" or "ci packages/web".
	Name string `json:"name"`
	// Branch is the branch the run is on (head_branch on the wire).
	Branch string `json:"head_branch"`
	// Status is "queued", "in_progress", or "completed".
	Status string `json:"status"`
	// Conclusion is the result once Status is completed: "success", "failure", "cancelled",
	// "timed_out", "skipped", "neutral", or "action_required". Empty until then.
	Conclusion string `json:"conclusion"`
	// URL is the run's address on the web.
	URL string `json:"html_url"`
	// StartedAt is when the run started, as an RFC 3339 string. Empty while it is queued.
	StartedAt string `json:"run_started_at"`
}

// Failed reports whether a run has completed and did not pass. A run still queued or in progress is
// not failed, and a run that was skipped or cancelled is not a failure to fix.
func (r WorkflowRun) Failed() bool {
	if r.Status != "completed" {
		return false
	}
	switch r.Conclusion {
	case "failure", "timed_out", "action_required":
		return true
	default:
		return false
	}
}

// Job is one job of a run, with its steps. It is what says which step failed, so the monitor can
// name it and read only that job's log.
type Job struct {
	// ID is the forge's numeric job id, which the log is read by.
	ID int64 `json:"id"`
	// Name is the job's name.
	Name string `json:"name"`
	// Conclusion is the job's result, "failure" and the rest as Conclusion on a run is.
	Conclusion string `json:"conclusion"`
	// Steps are the job's steps, in order.
	Steps []Step `json:"steps"`
}

// Failed reports whether the job failed.
func (j Job) Failed() bool {
	return j.Conclusion == "failure" || j.Conclusion == "timed_out" || j.Conclusion == "action_required"
}

// FailedStep names the first step that failed, and answers an empty name when the job has no step
// that did. A job can fail with no failed step (a cancelled or timed-out job often does), and the
// monitor then names the job itself rather than inventing a step.
func (j Job) FailedStep() string {
	for _, step := range j.Steps {
		if step.Conclusion == "failure" || step.Conclusion == "timed_out" {
			return step.Name
		}
	}
	return ""
}

// Step is one step of a job.
type Step struct {
	// Name is the step's name.
	Name string `json:"name"`
	// Conclusion is the step's result.
	Conclusion string `json:"conclusion"`
}

// ListWorkflowRuns lists the runs of a branch, newest first as the forge orders them
// (GET /repos/{owner}/{repo}/actions/runs?branch=).
func (c *caller) ListWorkflowRuns(ctx context.Context, repo Repository, branch string) ([]WorkflowRun, error) {
	if !repo.Valid() {
		return nil, errors.New("listing workflow runs needs a repository")
	}
	if strings.TrimSpace(branch) == "" {
		return nil, errors.New("listing workflow runs needs a branch")
	}
	var answer struct {
		Runs []WorkflowRun `json:"workflow_runs"`
	}
	path := c.repoPath(repo, "actions/runs") + "?branch=" + url.QueryEscape(branch)
	if err := c.do(ctx, http.MethodGet, path, nil, &answer); err != nil {
		return nil, err
	}
	if answer.Runs == nil {
		return []WorkflowRun{}, nil
	}
	return answer.Runs, nil
}

// ListRunJobs lists one run's jobs, with their steps
// (GET /repos/{owner}/{repo}/actions/runs/{run_id}/jobs).
func (c *caller) ListRunJobs(ctx context.Context, repo Repository, runID int64) ([]Job, error) {
	if !repo.Valid() {
		return nil, errors.New("listing a run's jobs needs a repository")
	}
	if runID <= 0 {
		return nil, errors.New("listing a run's jobs needs a positive run id")
	}
	var answer struct {
		Jobs []Job `json:"jobs"`
	}
	path := c.repoPath(repo, fmt.Sprintf("actions/runs/%d/jobs", runID))
	if err := c.do(ctx, http.MethodGet, path, nil, &answer); err != nil {
		return nil, err
	}
	if answer.Jobs == nil {
		return []Job{}, nil
	}
	return answer.Jobs, nil
}

// RerunFailedJobs asks the forge to run a run's failed jobs again
// (POST /repos/{owner}/{repo}/actions/runs/{run_id}/rerun-failed-jobs). It changes nothing about
// the run's id, which is why section 9's "rerun the failed jobs once" can be remembered on the
// run's own row.
func (c *caller) RerunFailedJobs(ctx context.Context, repo Repository, runID int64) error {
	if !repo.Valid() {
		return errors.New("rerunning failed jobs needs a repository")
	}
	if runID <= 0 {
		return errors.New("rerunning failed jobs needs a positive run id")
	}
	path := c.repoPath(repo, fmt.Sprintf("actions/runs/%d/rerun-failed-jobs", runID))
	return c.do(ctx, http.MethodPost, path, nil, nil)
}

// FailedLog reads the log of a run's first failed job, naming the job and its failed step, and cuts
// it to maxBytes. It reads only the failed job: a run's other jobs passed, so their output is not
// what the card's agent needs. An empty name means no job failed, and the answer is then empty
// rather than an error, because a run can be reported failed with no failed job to read.
//
// GitHub answers the log address with a redirect to a signed URL that needs no token, and the body
// there is a zip of one file per step; both are handled here so no caller has to know. The signed
// URL is fetched by the same HTTP client, which does not carry the Authorization header to another
// host.
func (c *caller) FailedLog(ctx context.Context, repo Repository, runID int64, maxBytes int) (string, error) {
	jobs, err := c.ListRunJobs(ctx, repo, runID)
	if err != nil {
		return "", err
	}
	var failed *Job
	for i := range jobs {
		if jobs[i].Failed() {
			failed = &jobs[i]
			break
		}
	}
	if failed == nil {
		return "", nil
	}
	text, err := c.jobLog(ctx, repo, failed.ID, maxBytes)
	if err != nil {
		return "", err
	}
	header := failed.Name
	if step := failed.FailedStep(); step != "" {
		header += " / " + step
	}
	return "# " + header + "\n" + text, nil
}

// jobLog reads one job's log and answers it as text. The forge may answer a zip of per-step files
// or the text itself, so both are accepted: a body that starts with the zip signature is unzipped
// and its files joined in order.
func (c *caller) jobLog(ctx context.Context, repo Repository, jobID int64, maxBytes int) (string, error) {
	if maxBytes <= 0 || maxBytes > maxLogBytes {
		maxBytes = maxLogBytes
	}
	path := c.repoPath(repo, fmt.Sprintf("actions/jobs/%d/logs", jobID))
	body, err := c.raw(ctx, http.MethodGet, path, maxBytes)
	if err != nil {
		return "", err
	}
	if isZip(body) {
		return unzipLogs(body, maxBytes)
	}
	return string(body), nil
}

// isZip reports whether a body is a zip archive by its signature.
func isZip(body []byte) bool {
	return len(body) >= 4 && body[0] == 'P' && body[1] == 'K' && body[2] == 3 && body[3] == 4
}

// unzipLogs joins a log archive's files in the order the archive lists them, cut to maxBytes. The
// archive is read from memory: it was already bounded when it was read.
func unzipLogs(body []byte, maxBytes int) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		// Not a readable archive: answer it as text rather than losing the only copy there is.
		return string(body), nil
	}
	var out strings.Builder
	for _, file := range reader.File {
		if out.Len() >= maxBytes {
			break
		}
		contents, err := readZipFile(file, maxBytes-out.Len())
		if err != nil {
			return "", err
		}
		if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
			out.WriteByte('\n')
		}
		out.WriteString(contents)
	}
	return out.String(), nil
}

// readZipFile reads one file out of an archive, reading at most maxBytes of it.
func readZipFile(file *zip.File, maxBytes int) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("read a log file out of the archive: %w", err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, int64(maxBytes)))
	if err != nil {
		return "", fmt.Errorf("read a log file out of the archive: %w", err)
	}
	return string(data), nil
}

// raw sends one request and answers its body, read up to maxBytes. It is for the answers that are
// not JSON - a job's log - and it follows the forge's redirect to a signed URL for us.
func (c *caller) raw(ctx context.Context, method, path string, maxBytes int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/"+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build a GitHub request: %w", err)
	}
	if c.sign != nil {
		c.sign(req)
	}
	req.Header.Set("Accept", AcceptHeader)
	req.Header.Set("X-GitHub-Api-Version", APIVersion)
	req.Header.Set("User-Agent", "Marshal")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call GitHub: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.apiError(resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)))
	if err != nil {
		return nil, fmt.Errorf("read a GitHub answer: %w", err)
	}
	return body, nil
}

// RunIDFromEvent reads a forge run id out of a delivery's numeric id, so a parser can turn the
// event's JSON number into the string the wire carries. It is here rather than in the parser so
// the one rule - a run id is the forge's number, written as text - lives with the forge.
func RunIDFromEvent(id int64) string { return strconv.FormatInt(id, 10) }
