package integrations

// This file is the GitHub connection test (docs/architecture.md section 18, B6.7, build-plan 6.8):
// what Marshal asks GitHub to prove an App is set up, and what it tells the person about each
// answer. It is one connection's half of the machinery in internal/connectiontest - the cooldown,
// the time limit, and the saving of the result all live there; all this file writes is the answer to
// "does this App work, and what can it see".
//
// The test is read-only. It makes three calls: the App's own installation (which only an App-signed
// request can read, so a matching answer proves the App's id and key are right), the repositories
// the installation can see, and - without calling GitHub at all - whether a delivery has reached
// Marshal's own route. The last one is what makes a blocked webhook visible: a secret the two sides
// disagree about looks exactly like a webhook GitHub never sent, and the only way to tell them apart
// is to look at what the route has actually seen.

import (
	"context"
	"errors"
	"fmt"

	ghclient "github.com/khanblair/marshal/daemon/internal/github"
	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The names of the checks the GitHub test reports. They are the row labels a screen shows, so they
// are words rather than ids.
const (
	// CheckSummary is the check whose message is the one sentence a connection row shows. Every
	// connection test names one check this, so a row reads back what the test found without the
	// list route knowing what any particular test asks.
	CheckSummary = "Summary"
	// CheckAppInstalled is the check that GitHub's own installation for this App is readable.
	CheckAppInstalled = "App installed"
	// CheckRepositories is the check that the installation can see at least one repository.
	CheckRepositories = "Repositories"
	// CheckPermissions is the check that the App may do what Marshal needs, by GitHub's own
	// permission names.
	CheckPermissions = "Permissions"
	// CheckWebhook is the check that a delivery has reached Marshal's own route and verified.
	CheckWebhook = "Webhook"
)

// requiredPermissions is what Marshal needs of the App, by GitHub's own permission names, and the
// level it needs. Marshal opens pull requests and comments on them, so issues and pull requests need
// write; it reruns failed CI jobs, so Actions needs write too. A permission GitHub reports with only
// read is a warning: the App works for reading what Marshal already has, and the person is told
// exactly which action will fail.
var requiredPermissions = []struct {
	Name  string
	Level string
}{
	{"issues", "write"},
	{"pull_requests", "write"},
	{"actions", "write"},
}

// Test runs the GitHub connection test and answers what to show the person. An id Marshal has no
// connection for is not found; everything else - an App GitHub refused, a key that does not work, a
// webhook that has not arrived - is part of the answer, not an error.
func (s *Service) Test(ctx context.Context, id string) (protocol.TestResult, error) {
	info, ok := Lookup(id)
	if !ok {
		return protocol.TestResult{}, protocol.NotFound("connection").With("id", id)
	}
	if !info.Wired {
		// A connection of a later phase has no test yet, and Marshal says so rather than making
		// one up. It is still a result, so the screen shows a row of checks like any other.
		return protocol.NewTestResult(id, []protocol.TestCheck{{
			Name:    CheckSummary,
			State:   protocol.CheckStateWarning,
			Message: fmt.Sprintf("Marshal cannot test %s yet.", info.ID),
			Fix:     "This connection is built in a later step.",
		}}, s.now()), nil
	}
	return s.test(ctx, info)
}

// testFor is the dispatcher the service uses when no Tester override was handed in: it picks the
// test that belongs to a connection's kind. A connection whose kind has no test yet is not reached
// here, because Test refuses a connection Marshal has not wired (see Info.Wired); this is only the
// list of kinds that have one.
func (s *Service) testFor(ctx context.Context, info Info) (protocol.TestResult, error) {
	switch info.Kind {
	case KindGitHub:
		return s.testGitHub(ctx, info)
	case KindObsidian:
		return s.testObsidian(ctx, info)
	default:
		return protocol.TestResult{}, protocol.NotFound("connection").With("id", info.ID)
	}
}

// testGitHub is the real GitHub test. It is behind Options.Tester so that no route test ever dials
// GitHub, and so the checks themselves are driven from a fake without a live App.
func (s *Service) testGitHub(ctx context.Context, info Info) (protocol.TestResult, error) {
	if info.ID != GitHubID {
		return protocol.TestResult{}, protocol.NotFound("connection").With("id", info.ID)
	}
	app, err := s.appFn(ctx)
	if errors.Is(err, ErrNotConnected) {
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name:    CheckSummary,
			State:   protocol.CheckStateFailed,
			Message: "Marshal has no GitHub App saved, so it cannot use GitHub.",
			Fix: "Add the App's id, installation id, private key, and webhook secret in Settings, " +
				"under Integrations.",
		}}, s.now()), nil
	}
	if err != nil {
		return protocol.TestResult{}, err
	}

	checks := make([]protocol.TestCheck, 0, 5)
	install, installErr := app.Installation(ctx)
	checks = append(checks, installationCheck(install, installErr))
	checks = append(checks, permissionsCheck(install, installErr))

	repos, reposErr := app.Repositories(ctx)
	checks = append(checks, repositoriesCheck(repos, reposErr))

	checks = append(checks, webhookCheck(s.webhooks.Deliveries()))
	checks = append([]protocol.TestCheck{summaryCheck(checks,
		"The GitHub App works and Marshal can use it.",
		"The GitHub App works, with something to check.")}, checks...)
	return protocol.NewTestResult(info.ID, checks, s.now()), nil
}

// installationCheck is what GitHub said about the App's own installation. It is the one call that
// only an App-signed request can make, so its answer is what proves the App's id and key are right.
func installationCheck(install githubapp.Installation, err error) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckAppInstalled}
	if err != nil {
		check.State, check.Message, check.Fix = githubFailure(err,
			"Marshal could not read this App's installation.")
		return check
	}
	where := install.Account
	if where == "" {
		where = "this account"
	}
	check.State = protocol.CheckStatePassed
	check.Message = fmt.Sprintf("The App is installed on %s.", where)
	return check
}

// permissionsCheck is what the App is allowed to do. A permission GitHub does not grant at all is a
// failure - the action cannot be attempted - and one granted only read where Marshal needs write is
// a warning naming what will fail.
func permissionsCheck(install githubapp.Installation, err error) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckPermissions}
	if err != nil {
		// Without the installation there are no permissions to read, and the installation check
		// already says why. This one is honest that it could not be answered.
		check.State = protocol.CheckStateWarning
		check.Message = "Marshal could not read the App's permissions, because it could not read the installation."
		return check
	}
	var missing, readOnly []string
	for _, want := range requiredPermissions {
		got := install.Permissions[want.Name]
		switch {
		case got == "":
			missing = append(missing, want.Name)
		case !permissionAtLeast(got, want.Level):
			readOnly = append(readOnly, fmt.Sprintf("%s (%s, needs %s)", want.Name, got, want.Level))
		}
	}
	switch {
	case len(missing) > 0:
		check.State = protocol.CheckStateFailed
		check.Message = fmt.Sprintf("The App is missing the %s permission.",
			joinWords(missing))
		check.Fix = "Add the missing permission to the GitHub App, then accept the change on the installation."
	case len(readOnly) > 0:
		check.State = protocol.CheckStateWarning
		check.Message = fmt.Sprintf("The App may only read %s, and Marshal writes them.",
			joinWords(readOnly))
		check.Fix = "Raise the permission to write on the GitHub App, then accept the change on the installation."
	default:
		check.State = protocol.CheckStatePassed
		check.Message = "The App may read and write issues, pull requests, and Actions."
	}
	return check
}

// permissionAtLeast reports whether a permission GitHub reported is at least the level Marshal
// needs. GitHub's own words are "read", "write", and "admin", in that order.
func permissionAtLeast(got, want string) bool {
	rank := func(level string) int {
		switch level {
		case "read":
			return 1
		case "write":
			return 2
		case "admin":
			return 3
		default:
			return 0
		}
	}
	return rank(got) >= rank(want) && rank(want) > 0
}

// repositoriesCheck is what the installation can see. None at all is a failure: Marshal cannot open
// a pull request in a repository it cannot see, which is what a person sees as "the App is installed
// on nothing".
func repositoriesCheck(repos []string, err error) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckRepositories}
	if err != nil {
		check.State, check.Message, check.Fix = githubFailure(err,
			"Marshal could not list the App's repositories.")
		return check
	}
	if len(repos) == 0 {
		check.State = protocol.CheckStateFailed
		check.Message = "The App can see no repositories."
		check.Fix = "On the GitHub App's installation, give it access to at least one repository."
		return check
	}
	check.State = protocol.CheckStatePassed
	check.Message = fmt.Sprintf("Marshal can see %s.", plural(len(repos), "repository", "repositories"))
	return check
}

// webhookCheck is whether a delivery has reached Marshal's own route. It never calls GitHub: what it
// reports is what the route has actually seen since the daemon started, which is the only way to
// tell a secret the two sides disagree about from a webhook GitHub never sent.
//
// It is a warning and not a failure when nothing has arrived, because GitHub sends a ping only when
// the webhook is created or a delivery is asked for by hand - a fresh daemon with a correct setup
// has simply not been sent one yet. A delivery that arrived and was refused is a failure: that is
// exactly the blocked webhook the check exists to catch.
func webhookCheck(seen githubapp.DeliveryReading) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckWebhook}
	switch {
	case seen.Refused > 0 && seen.LastError != nil:
		check.State = protocol.CheckStateFailed
		switch {
		case errors.Is(seen.LastError, githubapp.ErrNoSecret):
			check.Message = "A delivery arrived but Marshal has no webhook secret saved, so it could not check it."
			check.Fix = "Save the webhook secret from the GitHub App's settings."
		case errors.Is(seen.LastError, githubapp.ErrBadSignature),
			errors.Is(seen.LastError, githubapp.ErrNoSignature):
			check.Message = "A delivery arrived with a signature Marshal could not match."
			check.Fix = "Check that the webhook secret is the same on GitHub and in Marshal."
		default:
			check.Message = fmt.Sprintf("A delivery arrived that Marshal could not accept: %s.", seen.LastError)
		}
	case seen.Accepted > 0:
		check.State = protocol.CheckStatePassed
		check.Message = fmt.Sprintf("A delivery reached Marshal and verified%s.",
			kindPhrase(seen.LastKind))
	default:
		check.State = protocol.CheckStateWarning
		check.Message = "No delivery has reached Marshal since it started."
		check.Fix = "In the App's webhook settings, use Redeliver, or press Test again after GitHub sends a ping."
	}
	return check
}

// kindPhrase names the newest event in a parenthetical, or nothing when its kind was not readable.
func kindPhrase(kind string) string {
	if kind == "" {
		return ""
	}
	return " (the newest was a " + kind + ")"
}

// summaryCheck is the one sentence the connection row shows, built from the checks themselves: the
// first failure if there is one, and otherwise a short account of what was proven. The two
// sentences are the caller's, because only the caller knows what its own test proved.
func summaryCheck(checks []protocol.TestCheck, works, partly string) protocol.TestCheck {
	for _, check := range checks {
		if check.State == protocol.CheckStateFailed {
			return protocol.TestCheck{
				Name:    CheckSummary,
				State:   protocol.CheckStateFailed,
				Message: check.Message,
				Fix:     check.Fix,
			}
		}
	}
	warned := 0
	for _, check := range checks {
		if check.State == protocol.CheckStateWarning {
			warned++
		}
	}
	summary := protocol.TestCheck{Name: CheckSummary, State: protocol.CheckStatePassed}
	switch warned {
	case 0:
		summary.Message = works
	default:
		summary.State = protocol.CheckStateWarning
		summary.Message = partly
	}
	return summary
}

// githubFailure turns a failed GitHub call into the sentence and the fix a person reads. A refusal
// names the status, because "GitHub refused this" and "Marshal could not ask" are different problems
// with different fixes, and an API error's own text names the API, which a screen does not.
func githubFailure(err error, what string) (state protocol.CheckState, message, fix string) {
	var apiErr *ghclient.APIError
	switch {
	case errors.As(err, &apiErr) && apiErr.Unauthorized():
		return protocol.CheckStateFailed,
			"GitHub refused Marshal's App credentials.",
			"Check the App id, the installation id, and the private key, and save them again."
	case errors.As(err, &apiErr) && apiErr.NotFound():
		return protocol.CheckStateFailed,
			"GitHub has no such installation for this App.",
			"Check the installation id on the App's installation page."
	case errors.As(err, &apiErr):
		return protocol.CheckStateFailed,
			fmt.Sprintf("%s GitHub answered %d.", what, apiErr.Status),
			"Check the App's settings on GitHub."
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return protocol.CheckStateFailed,
			"GitHub did not answer in time.",
			"Check this computer's connection, then test again."
	default:
		return protocol.CheckStateFailed,
			fmt.Sprintf("%s Marshal could not reach GitHub.", what),
			"Check this computer's connection, then test again."
	}
}

// plural writes a count with its word, so a sentence reads "3 repositories" and not "3 repository".
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// joinWords joins names into a sentence list: "a", "a and b", "a, b, and c".
func joinWords(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	case 2:
		return words[0] + " and " + words[1]
	default:
		out := ""
		for i, word := range words {
			switch {
			case i == len(words)-1:
				out += ", and " + word
			case i == 0:
				out = word
			default:
				out += ", " + word
			}
		}
		return out
	}
}
