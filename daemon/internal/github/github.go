// Package github is Marshal's client for talking to a Git forge (docs/architecture.md section 8,
// docs/backend-checklist.md B5.4 and B5.6, build-plan 5.6 and 5.7). It is the only place Marshal
// speaks GitHub's HTTP API, the way internal/gitx is the only place it speaks Git: a caller depends
// on the Client interface and never on a token, a URL, or a JSON body.
//
// Phase 5 ships one implementation, TokenClient, which signs every request with a personal access
// token the owner saved (the token lives in the keychain, internal/secrets). Phase 6 adds a second
// implementation for the real GitHub App, behind the same interface, so nothing written here needs
// to change when the App arrives. That boundary is a ruling: a caller holds a Client, never a
// *TokenClient, exactly as a caller of gitx never touches exec.Cmd.
//
// Nothing here signs a person in and nothing here creates a real pull request on its own: the
// client is a plain, tested HTTP adapter, and the tests drive it against a fake server. A caller
// that opens a real pull request is the pull-request service, and the owner does the sign-in
// (docs/backend-checklist.md section 3).
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is GitHub's public API. A GitHub Enterprise address is set with WithBaseURL.
const DefaultBaseURL = "https://api.github.com"

// AcceptHeader is the media type every request asks for, so the answer's shape is the documented
// one and not whatever the server would otherwise default to.
const AcceptHeader = "application/vnd.github+json"

// APIVersion is the pinned GitHub API version, sent as X-GitHub-Api-Version. Pinning it keeps an
// answer's shape from changing under Marshal when GitHub rolls a new default.
const APIVersion = "2022-11-28"

// maxErrorBody bounds how much of a failed answer is read, so a server that answers with a whole
// document does not fill memory over it.
const maxErrorBody = 8 << 10

// Repository is the forge's own name for a repository: the account that owns it and the
// repository's name. It is read from the project's origin remote, never guessed.
type Repository struct {
	// Owner is the user or organization the repository lives under.
	Owner string
	// Name is the repository's name, without the ".git" suffix.
	Name string
}

// Valid reports whether the repository names both halves.
func (r Repository) Valid() bool {
	return strings.TrimSpace(r.Owner) != "" && strings.TrimSpace(r.Name) != ""
}

// String is the "owner/name" form, for messages and paths.
func (r Repository) String() string { return r.Owner + "/" + r.Name }

// RepositoryFromURL reads owner and name out of a GitHub address in any of the forms a clone remote
// can take: https://github.com/owner/repo(.git), ssh://git@github.com/owner/repo(.git), and
// git@github.com:owner/repo(.git). Any other host is not a GitHub repository, so it answers false.
//
// It lives with the forge's own types rather than with one caller because two of them read a
// project's origin remote (the pull-request service and the review that follows it) and they must
// always agree on which repository a card's work belongs to.
func RepositoryFromURL(raw string) (Repository, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Repository{}, false
	}
	var host, path string
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return Repository{}, false
		}
		host, path = parsed.Hostname(), parsed.Path
	} else {
		// The scp form, git@host:owner/repo.
		at := strings.LastIndex(raw, "@")
		colon := strings.Index(raw, ":")
		if at < 0 || colon < 0 || colon < at {
			return Repository{}, false
		}
		host, path = raw[at+1:colon], raw[colon+1:]
	}
	if !isGitHubHost(host) {
		return Repository{}, false
	}
	path = strings.Trim(strings.TrimSuffix(path, ".git"), "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return Repository{}, false
	}
	owner, name := parts[len(parts)-2], parts[len(parts)-1]
	repo := Repository{Owner: owner, Name: name}
	if !repo.Valid() {
		return Repository{}, false
	}
	return repo, true
}

// isGitHubHost reports whether a host is GitHub itself or a GitHub Enterprise host, which is any
// host that names github in it. A GitHub Enterprise address is the only other place the token
// would work, and a client's base URL is what decides where the call really goes.
func isGitHubHost(host string) bool {
	return strings.Contains(strings.ToLower(host), "github")
}

// PullRequest is a pull request as the forge reports it.
type PullRequest struct {
	// Number is the pull request's number, which a person reads and a URL/route addresses.
	Number int `json:"number"`
	// URL is the pull request's own address on the web (GitHub's html_url).
	URL string `json:"html_url"`
	// State is "open", "closed", or "merged" as the forge words it.
	State string `json:"state"`
	// Title is the pull request's title.
	Title string `json:"title"`
	// Head is the branch the work is on.
	Head string `json:"-"`
	// Base is the branch it is being merged into.
	Base string `json:"-"`
}

// NewPullRequest is what a caller asks the forge to open.
type NewPullRequest struct {
	// Repo is the repository the pull request is in.
	Repo Repository
	// Title is the pull request's title, usually the card's title.
	Title string
	// Body is the description. It may be empty.
	Body string
	// Head is the branch the work is on (the card's branch).
	Head string
	// Base is the branch to merge into (the project's default branch).
	Base string
	// Draft opens the pull request as a draft.
	Draft bool
}

// ReviewComment is a comment on a pull request or on one line of its diff.
type ReviewComment struct {
	// ID is the comment's own id at the forge.
	ID int64 `json:"id"`
	// URL is the comment's address on the web.
	URL string `json:"html_url"`
	// Body is the comment's text.
	Body string `json:"body"`
	// Path is the file the comment is on, empty for a comment on the pull request itself.
	Path string `json:"path"`
	// Line is the line the comment is on, 0 when it is not on one.
	Line int `json:"line"`
	// CommitSHA is the commit the comment is on, empty for a comment on the pull request itself.
	CommitSHA string `json:"-"`
}

// NewReviewComment is a comment a caller asks the forge to post. When Path is empty the comment is
// posted on the pull request itself (the issue comment endpoint); otherwise it is posted on that
// line of the diff. Either way it is what the card's worker reads.
type NewReviewComment struct {
	// Repo is the repository the pull request is in.
	Repo Repository
	// Number is the pull request's number.
	Number int
	// Body is the comment's text. It cannot be empty.
	Body string
	// Path is the file to comment on. Empty posts a comment on the pull request itself.
	Path string
	// Line is the line in the file's diff. Used only when Path is set.
	Line int
	// Side is "RIGHT" for the new side of the diff or "LEFT" for the old one. Empty means RIGHT.
	Side string
	// CommitSHA pins a diff comment to a commit. Empty means the newest commit.
	CommitSHA string
}

// Check is one check run on a commit, as the forge reports it: the workflow or job's name, its
// status ("queued", "in_progress", "completed"), and its conclusion ("success", "failure",
// "neutral", "cancelled", "skipped", "timed_out", "action_required"). It is what the merge queue
// reads before it lets a target branch move.
type Check struct {
	// Name is the check's name.
	Name string `json:"name"`
	// Status is the check's status.
	Status string `json:"status"`
	// Conclusion is the check's result, empty until it is completed.
	Conclusion string `json:"conclusion"`
	// URL is the check's address on the web.
	URL string `json:"html_url"`
}

// Passed reports whether a check has completed successfully. Only "success" passes; "neutral" and
// "skipped" are not a pass, because a check that did not run must not be read as green.
func (c Check) Passed() bool {
	return c.Status == "completed" && c.Conclusion == "success"
}

// Client is how Marshal talks to a Git forge. A caller builds a pull request, a review comment, or
// a check request and the implementation makes the HTTP call. Returning a *TokenClient where a
// Client is wanted is the point: Phase 6's App implementation goes behind the same interface.
type Client interface {
	// CreatePullRequest opens a pull request and returns it as the forge made it.
	CreatePullRequest(ctx context.Context, req NewPullRequest) (PullRequest, error)
	// GetPullRequest reads one pull request.
	GetPullRequest(ctx context.Context, repo Repository, number int) (PullRequest, error)
	// CreateReviewComment posts a comment on a pull request or on one line of its diff.
	CreateReviewComment(ctx context.Context, req NewReviewComment) (ReviewComment, error)
	// ListChecks lists the check runs on a ref (a branch name or a commit SHA).
	ListChecks(ctx context.Context, repo Repository, ref string) ([]Check, error)
}

var (
	_ Client = (*TokenClient)(nil)
	_ Client = (*TransportClient)(nil)
)

// caller is the half of a client that speaks GitHub's HTTP: where the API lives, the HTTP client
// to use, and the clock. Authorization is the one thing the Phase 5 personal-token client and
// Phase 6's App client do differently, so it is a hook here and every request shape above is
// written once, in this package, and used by both. That is the package's whole claim: it is the
// only place Marshal speaks GitHub's API.
type caller struct {
	baseURL string
	http    *http.Client
	// now is the clock, so a test can prove a timeout is made from it rather than from the wall.
	now func() time.Time
	// sign adds whatever a request needs to be authorized. It is nil for a client whose
	// authorization is done by its HTTP transport (the App's installation token), and sets the
	// Authorization header for a personal token.
	sign func(*http.Request)
}

// TokenClient is a Client that signs every request with a personal access token. It is the Phase 5
// implementation, for the owner's personal sign-in. It is safe for use by many goroutines: it
// holds no mutable state after it is built.
type TokenClient struct {
	*caller
}

// TransportClient is a Client whose requests are authorized by its own HTTP transport rather than
// by a header this package sets. It is how Phase 6's GitHub App plugs in behind the same
// interface: ghinstallation provides a transport that signs each request with a fresh installation
// token, and this client speaks exactly the HTTP the rest of this package speaks. A caller never
// builds one by hand; internal/integrations/github builds it from an App's key.
type TransportClient struct {
	*caller
}

// Option changes how a client is built. It is shared by NewTokenClient and NewTransportClient.
type Option func(*caller)

// WithBaseURL points the client at a GitHub Enterprise address (or, in a test, a fake server).
// A trailing slash is trimmed so paths join cleanly.
func WithBaseURL(base string) Option {
	return func(c *caller) {
		if base != "" {
			c.baseURL = strings.TrimRight(base, "/")
		}
	}
}

// WithHTTPClient sets the HTTP client, so a test can use one with a transport of its own.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *caller) {
		if hc != nil {
			c.http = hc
		}
	}
}

// WithTimeout sets the per-request timeout. A caller's own deadline still applies; this is the
// shorter of the two. The default is twenty seconds, the same limit connection tests use.
func WithTimeout(d time.Duration) Option {
	return func(c *caller) {
		if d > 0 {
			c.http.Timeout = d
		}
	}
}

// WithClock sets the clock. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(c *caller) {
		if now != nil {
			c.now = now
		}
	}
}

// newCaller builds the shared half of a client and applies the options. A nil HTTP client means
// the default one, with the twenty-second timeout every outbound call in Marshal uses.
func newCaller(hc *http.Client, opts []Option) *caller {
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	c := &caller{baseURL: DefaultBaseURL, http: hc, now: time.Now}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewTokenClient builds a personal-token client. An empty token is refused: a client with no token
// would answer every call with a 401, which is a confusing way to discover a missing key.
func NewTokenClient(token string, opts ...Option) (*TokenClient, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("a GitHub token is required")
	}
	c := newCaller(nil, opts)
	c.sign = func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return &TokenClient{caller: c}, nil
}

// NewTransportClient builds a client whose requests are authorized by the given HTTP client's own
// transport. It is how a GitHub App is used: the transport signs each request with an installation
// token, so this client adds no Authorization header of its own. A nil HTTP client is refused
// rather than defaulting to an unauthenticated one, which would answer every call with a 401.
func NewTransportClient(hc *http.Client, opts ...Option) (*TransportClient, error) {
	if hc == nil {
		return nil, errors.New("a GitHub App client needs an HTTP client that signs its requests")
	}
	c := newCaller(hc, opts)
	c.sign = nil
	return &TransportClient{caller: c}, nil
}

// CreatePullRequest opens a pull request (POST /repos/{owner}/{repo}/pulls).
func (c *caller) CreatePullRequest(ctx context.Context, req NewPullRequest) (PullRequest, error) {
	if !req.Repo.Valid() {
		return PullRequest{}, errors.New("a pull request needs a repository")
	}
	if strings.TrimSpace(req.Title) == "" {
		return PullRequest{}, errors.New("a pull request needs a title")
	}
	if strings.TrimSpace(req.Head) == "" || strings.TrimSpace(req.Base) == "" {
		return PullRequest{}, errors.New("a pull request needs a head branch and a base branch")
	}
	body := map[string]any{
		"title": req.Title,
		"head":  req.Head,
		"base":  req.Base,
		"draft": req.Draft,
	}
	if req.Body != "" {
		body["body"] = req.Body
	}
	var pr pullRequestPayload
	if err := c.do(ctx, http.MethodPost, c.repoPath(req.Repo, "pulls"), body, &pr); err != nil {
		return PullRequest{}, err
	}
	return pr.toPullRequest(), nil
}

// GetPullRequest reads one pull request (GET /repos/{owner}/{repo}/pulls/{number}).
func (c *caller) GetPullRequest(ctx context.Context, repo Repository, number int) (PullRequest, error) {
	if !repo.Valid() {
		return PullRequest{}, errors.New("a pull request needs a repository")
	}
	if number <= 0 {
		return PullRequest{}, errors.New("a pull request needs a positive number")
	}
	var pr pullRequestPayload
	if err := c.do(ctx, http.MethodGet, c.repoPath(repo, fmt.Sprintf("pulls/%d", number)), nil, &pr); err != nil {
		return PullRequest{}, err
	}
	return pr.toPullRequest(), nil
}

// CreateReviewComment posts a comment on a pull request or on one line of its diff. A comment with
// no path is posted on the pull request itself, which is where the Reviewer's summary goes; one
// with a path is posted on that line of the diff.
func (c *caller) CreateReviewComment(ctx context.Context, req NewReviewComment) (ReviewComment, error) {
	if !req.Repo.Valid() {
		return ReviewComment{}, errors.New("a review comment needs a repository")
	}
	if req.Number <= 0 {
		return ReviewComment{}, errors.New("a review comment needs a positive pull request number")
	}
	if strings.TrimSpace(req.Body) == "" {
		return ReviewComment{}, errors.New("a review comment needs a body")
	}
	if strings.TrimSpace(req.Path) == "" {
		body := map[string]any{"body": req.Body}
		var comment ReviewComment
		path := c.repoPath(req.Repo, fmt.Sprintf("issues/%d/comments", req.Number))
		if err := c.do(ctx, http.MethodPost, path, body, &comment); err != nil {
			return ReviewComment{}, err
		}
		return comment, nil
	}
	side := strings.ToUpper(strings.TrimSpace(req.Side))
	if side != "LEFT" {
		side = "RIGHT"
	}
	body := map[string]any{"body": req.Body, "path": req.Path, "side": side}
	if req.Line > 0 {
		body["line"] = req.Line
	}
	if req.CommitSHA != "" {
		body["commit_id"] = req.CommitSHA
	}
	var comment ReviewComment
	path := c.repoPath(req.Repo, fmt.Sprintf("pulls/%d/comments", req.Number))
	if err := c.do(ctx, http.MethodPost, path, body, &comment); err != nil {
		return ReviewComment{}, err
	}
	return comment, nil
}

// ListChecks lists the check runs on a ref (GET /repos/{owner}/{repo}/commits/{ref}/check-runs).
func (c *caller) ListChecks(ctx context.Context, repo Repository, ref string) ([]Check, error) {
	if !repo.Valid() {
		return nil, errors.New("listing checks needs a repository")
	}
	if strings.TrimSpace(ref) == "" {
		return nil, errors.New("listing checks needs a ref")
	}
	var answer struct {
		CheckRuns []Check `json:"check_runs"`
	}
	path := c.repoPath(repo, "commits/"+url.PathEscape(ref)+"/check-runs")
	if err := c.do(ctx, http.MethodGet, path, nil, &answer); err != nil {
		return nil, err
	}
	if answer.CheckRuns == nil {
		return []Check{}, nil
	}
	return answer.CheckRuns, nil
}

// pullRequestPayload is what GitHub answers with for a pull request. The head and base are nested
// objects on the wire and flatten into the PullRequest the caller reads.
type pullRequestPayload struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
	Title   string `json:"title"`
	Merged  bool   `json:"merged"`
	Head    struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

// toPullRequest flattens the payload into the shape a caller holds.
func (p pullRequestPayload) toPullRequest() PullRequest {
	return PullRequest{
		Number: p.Number, URL: p.HTMLURL, State: p.State, Title: p.Title,
		Head: p.Head.Ref, Base: p.Base.Ref,
	}
}

// repoPath builds the "repos/{owner}/{repo}/..." path, escaping the two names.
func (c *caller) repoPath(repo Repository, tail string) string {
	return "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name) + "/" + tail
}

// do sends one request, decodes a successful answer into out (when out is not nil), and turns a
// failed one into an *APIError. `body` is JSON-encoded when it is not nil.
func (c *caller) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode a GitHub request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/"+path, reader)
	if err != nil {
		return fmt.Errorf("build a GitHub request: %w", err)
	}
	if c.sign != nil {
		c.sign(req)
	}
	req.Header.Set("Accept", AcceptHeader)
	req.Header.Set("X-GitHub-Api-Version", APIVersion)
	req.Header.Set("User-Agent", "Marshal")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call GitHub: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.apiError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("read a GitHub answer: %w", err)
	}
	return nil
}

// apiError reads a failed answer's message and turns it into an *APIError. The token is never
// repeated in the error, and the body is bounded.
func (c *caller) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	var payload struct {
		Message string `json:"message"`
		DocURL  string `json:"documentation_url"`
	}
	_ = json.Unmarshal(body, &payload)
	message := strings.TrimSpace(payload.Message)
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		message = resp.Status
	}
	return &APIError{
		Status:    resp.StatusCode,
		Message:   message,
		DocURL:    payload.DocURL,
		RequestID: resp.Header.Get("X-GitHub-Request-Id"),
	}
}

// APIError is a failed GitHub call: the HTTP status, the forge's own message, and where to read
// more. Status is what a caller switches on (401 for a bad token, 404 for a repository or pull
// request that is not there, 422 for a request the forge refused).
type APIError struct {
	// Status is the HTTP status code.
	Status int
	// Message is the forge's message.
	Message string
	// DocURL is the address of the documentation for the failure, when the forge gave one.
	DocURL string
	// RequestID is GitHub's request id, which is what support asks for.
	RequestID string
}

// Error is the plain sentence a person reads.
func (e *APIError) Error() string {
	if e.DocURL != "" {
		return fmt.Sprintf("GitHub answered %d: %s (%s)", e.Status, e.Message, e.DocURL)
	}
	return fmt.Sprintf("GitHub answered %d: %s", e.Status, e.Message)
}

// Unauthorized reports whether the failure was a 401: the token is missing, expired, or lacks the
// permission for what was asked. It is the one failure worth naming in the UI as "sign in again".
func (e *APIError) Unauthorized() bool { return e.Status == http.StatusUnauthorized }

// NotFound reports whether the failure was a 404.
func (e *APIError) NotFound() bool { return e.Status == http.StatusNotFound }

// Is lets errors.Is match on an APIError's status through the sentinels below.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrUnauthorized:
		return e.Unauthorized()
	case ErrNotFound:
		return e.NotFound()
	}
	return false
}

// ErrUnauthorized is matched by errors.Is when a call failed with a 401.
var ErrUnauthorized = errors.New("GitHub refused the token")

// ErrNotFound is matched by errors.Is when a call failed with a 404.
var ErrNotFound = errors.New("GitHub has no such thing")

// Status returns the HTTP status of an error, or 0 when it is not an *APIError.
func Status(err error) int {
	var api *APIError
	if errors.As(err, &api) {
		return api.Status
	}
	return 0
}
