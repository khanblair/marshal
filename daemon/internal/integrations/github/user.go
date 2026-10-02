package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	ghclient "github.com/khanblair/marshal/daemon/internal/github"
)

// UserAPI reads the parts of GitHub's API that describe who a token is and what it can reach. Every
// call is signed with the token it is given, so one value serves any number of connections.
type UserAPI struct {
	baseURL string
	http    *http.Client
}

// NewUserAPI builds the reader. Empty baseURL means GitHub's public API, nil hc a 20 second client.
func NewUserAPI(baseURL string, hc *http.Client) *UserAPI {
	if baseURL == "" {
		baseURL = ghclient.DefaultBaseURL
	}
	if hc == nil {
		hc = &http.Client{Timeout: DefaultTimeout}
	}
	return &UserAPI{baseURL: strings.TrimRight(baseURL, "/"), http: hc}
}

// Identity is who a token belongs to.
type Identity struct {
	Login string
	// Scopes is what a classic token was granted. ScopesKnown is false for tokens GitHub does not
	// list scopes for, such as fine-grained ones and App user tokens.
	Scopes      []string
	ScopesKnown bool
}

// HasScope reports whether a classic token carries one scope.
func (i Identity) HasScope(scope string) bool {
	for _, got := range i.Scopes {
		if got == scope {
			return true
		}
	}
	return false
}

// UserInstallation is one account the GitHub App is installed on, as the signed-in user sees it.
type UserInstallation struct {
	ID              int64
	Account         string
	Kind            string
	AllRepositories bool
	Permissions     map[string]string
}

// Whoami reads the user a token belongs to (GET /user).
func (u *UserAPI) Whoami(ctx context.Context, token string) (Identity, error) {
	var answer struct {
		Login string `json:"login"`
	}
	header, err := u.get(ctx, token, "/user", &answer)
	if err != nil {
		return Identity{}, err
	}
	id := Identity{Login: answer.Login}
	if raw, ok := header["X-Oauth-Scopes"]; ok && len(raw) > 0 {
		id.ScopesKnown = true
		for _, scope := range strings.Split(raw[0], ",") {
			if scope = strings.TrimSpace(scope); scope != "" {
				id.Scopes = append(id.Scopes, scope)
			}
		}
	}
	return id, nil
}

// Installations lists the accounts the App is installed on that this user can reach.
func (u *UserAPI) Installations(ctx context.Context, token string) ([]UserInstallation, error) {
	var answer struct {
		Installations []struct {
			ID      int64 `json:"id"`
			Account struct {
				Login string `json:"login"`
				Type  string `json:"type"`
			} `json:"account"`
			RepositorySelection string            `json:"repository_selection"`
			Permissions         map[string]string `json:"permissions"`
		} `json:"installations"`
	}
	if _, err := u.get(ctx, token, "/user/installations?per_page=100", &answer); err != nil {
		return nil, err
	}
	out := make([]UserInstallation, 0, len(answer.Installations))
	for _, in := range answer.Installations {
		kind := "user"
		if strings.EqualFold(in.Account.Type, "Organization") {
			kind = "organization"
		}
		out = append(out, UserInstallation{
			ID: in.ID, Account: in.Account.Login, Kind: kind,
			AllRepositories: in.RepositorySelection == "all", Permissions: in.Permissions,
		})
	}
	return out, nil
}

// InstallationRepositories counts the repositories one installation lets this user reach.
func (u *UserAPI) InstallationRepositories(ctx context.Context, token string, installationID int64) (int, error) {
	var answer struct {
		TotalCount int `json:"total_count"`
	}
	path := "/user/installations/" + strconv.FormatInt(installationID, 10) + "/repositories?per_page=1"
	if _, err := u.get(ctx, token, path, &answer); err != nil {
		return 0, err
	}
	return answer.TotalCount, nil
}

// HasRepositories reports whether a token can list at least one repository (GET /user/repos).
func (u *UserAPI) HasRepositories(ctx context.Context, token string) (bool, error) {
	var answer []struct {
		FullName string `json:"full_name"`
	}
	if _, err := u.get(ctx, token, "/user/repos?per_page=1", &answer); err != nil {
		return false, err
	}
	return len(answer) > 0, nil
}

func (u *UserAPI) get(ctx context.Context, token, path string, out any) (http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build the request for %s: %w", path, err)
	}
	req.Header.Set("Accept", ghclient.AcceptHeader)
	req.Header.Set("X-GitHub-Api-Version", ghclient.APIVersion)
	req.Header.Set("Authorization", "Bearer "+token)
	reply, err := u.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reply.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(reply.Body, maxReadBody))
	if err != nil {
		return nil, fmt.Errorf("read GitHub's answer: %w", err)
	}
	if reply.StatusCode < 200 || reply.StatusCode > 299 {
		return nil, &ghclient.APIError{
			Status: reply.StatusCode, Message: messageOf(body), RequestID: reply.Header.Get("X-GitHub-Request-Id"),
		}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return nil, fmt.Errorf("read GitHub's answer: %w", err)
	}
	return reply.Header, nil
}
