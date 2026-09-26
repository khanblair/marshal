package fixture

import (
	"context"
	"fmt"
	"time"

	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The prototype's 29 cards, written from the design's own seed data
// (design/store.js and apps/web/src/mock/seed/cards-*.ts). They are what makes the screens look
// the same against the daemon as they did against the mock, and what the end-to-end specs open.
//
// The values are the prototype's: the numbers (api goes to 46, and 41 is "Fix token refresh on
// login"), the states, the roles, the agents and models, the labels, the packages, the dates, the
// pull requests, the CI states, the "doing now" lines, the needs-you reasons, and the context.
//
// What is deliberately not here, because the daemon owns it or a later phase builds it: cost and
// token use (Phase 4), dependencies, members, checklists, and comments (their own phases), and the
// card's branch (a branch is made when a card starts, and a branch written now would stop that
// card's worktree being added later).
type prototypeCard struct {
	Project           string
	Number            int
	Title             string
	State             protocol.CardState
	Role              string
	Agent             protocol.AgentKind
	Model             string
	Thinking          protocol.ThinkingMode
	Perm              protocol.PermissionMode
	Package           string
	Doing             string
	Reason            string
	ReasonKind        protocol.NeedsReasonKind
	Context           int
	StartsInDays      int
	EndsInDays        int
	HasStart          bool
	HasEnd            bool
	DueInDays         int
	HasDue            bool
	PullNumber        int
	CI                protocol.CIState
	Pinned            bool
	Paused            bool
	Labels            []string
	UpdatedMinutesAgo int
}

// prototypeCards lists the prototype's cards, in the prototype's own order.
//
// The table is data, not code: the same short words ("Worker", "passed", "backlog", the project
// ids) repeat because 29 cards share them, and naming each one a constant would make the table
// harder to read against design/store.js. The mock's own seed file carries the same kind of
// exemption (`apps/web/src/mock/seed/cards-api.ts`).
//
//nolint:goconst // the prototype's own data table
func prototypeCards() []prototypeCard {
	return []prototypeCard{
		{Project: "api", Number: 41, Title: "Fix token refresh on login", State: "working", Role: "Worker", Agent: "claude", Model: "claude-sonnet-4-5", Thinking: "high", Perm: "auto-edits", Package: "", Doing: "Running auth tests", Reason: "", ReasonKind: "", Context: 46, StartsInDays: -2, EndsInDays: 1, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 0, CI: "running", Pinned: false, Paused: false, Labels: []string{"auth", "bug"}, UpdatedMinutesAgo: 1},
		{Project: "api", Number: 43, Title: "Add rate limiting per API key", State: "needs", Role: "Worker", Agent: "claude", Model: "claude-opus-4-1", Thinking: "extra-high", Perm: "plan", Package: "", Doing: "", Reason: "Plan ready for review", ReasonKind: "plan-ready", Context: 18, StartsInDays: 0, EndsInDays: 4, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 0, CI: "", Pinned: false, Paused: false, Labels: []string{"feature"}, UpdatedMinutesAgo: 34},
		{Project: "api", Number: 44, Title: "Upgrade grpc-go to 1.66", State: "needs", Role: "Worker", Agent: "codex", Model: "gpt-5-codex", Thinking: "medium", Perm: "ask", Package: "", Doing: "", Reason: "Approval needed: run go get google.golang.org/grpc@v1.66.0", ReasonKind: "approval-needed", Context: 22, StartsInDays: -1, EndsInDays: 1, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 0, CI: "", Pinned: false, Paused: false, Labels: []string{"deps"}, UpdatedMinutesAgo: 12},
		{Project: "api", Number: 39, Title: "Structured logging with slog", State: "review", Role: "Worker", Agent: "gemini", Model: "gemini-2.5-pro", Thinking: "medium", Perm: "auto-edits", Package: "", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -6, EndsInDays: -1, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 281, CI: "passed", Pinned: false, Paused: false, Labels: []string{"refactor"}, UpdatedMinutesAgo: 16},
		{Project: "api", Number: 40, Title: "Retry upstream calls with jitter", State: "review", Role: "Worker", Agent: "codex", Model: "gpt-5-codex", Thinking: "high", Perm: "full-auto", Package: "", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -4, EndsInDays: 0, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 284, CI: "running", Pinned: false, Paused: false, Labels: []string{"reliability"}, UpdatedMinutesAgo: 8},
		{Project: "api", Number: 36, Title: "Remove deprecated v1 routes", State: "ready", Role: "Worker", Agent: "claude", Model: "claude-sonnet-4-5", Thinking: "low", Perm: "auto-edits", Package: "", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -8, EndsInDays: -2, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 276, CI: "passed", Pinned: false, Paused: false, Labels: nil, UpdatedMinutesAgo: 17},
		{Project: "api", Number: 35, Title: "Health check returns build info", State: "merging", Role: "Integrator", Agent: "claude", Model: "claude-opus-4-1", Thinking: "high", Perm: "full-auto", Package: "", Doing: "Running affected tests after merge", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -7, EndsInDays: 0, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 274, CI: "passed", Pinned: false, Paused: false, Labels: nil, UpdatedMinutesAgo: 2},
		{Project: "api", Number: 33, Title: "Cache JWKS keys for 10 minutes", State: "done", Role: "Worker", Agent: "builtin", Model: "gpt-5-mini", Thinking: "low", Perm: "full-auto", Package: "", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -12, EndsInDays: -9, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 268, CI: "passed", Pinned: false, Paused: false, Labels: nil, UpdatedMinutesAgo: 2880},
		{Project: "api", Number: 45, Title: "OpenAPI spec for admin routes", State: "backlog", Role: "Docs writer", Agent: "builtin", Model: "deepseek-chat", Thinking: "medium", Perm: "auto-edits", Package: "", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: 3, EndsInDays: 6, HasStart: true, HasEnd: true, DueInDays: 7, HasDue: true, PullNumber: 0, CI: "", Pinned: false, Paused: false, Labels: []string{"docs"}, UpdatedMinutesAgo: 180},
		{Project: "api", Number: 46, Title: "Split config loader into packages", State: "planning", Role: "Worker", Agent: "claude", Model: "claude-sonnet-4-5", Thinking: "high", Perm: "plan", Package: "", Doing: "Reading internal/config and its callers", Reason: "", ReasonKind: "", Context: 22, StartsInDays: 1, EndsInDays: 5, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 0, CI: "", Pinned: false, Paused: false, Labels: nil, UpdatedMinutesAgo: 1},
		{Project: "api", Number: 42, Title: "Load test the /v2/proxy path", State: "working", Role: "Tester", Agent: "builtin", Model: "gpt-5-mini", Thinking: "low", Perm: "full-auto", Package: "", Doing: "Running k6 at 500 requests per second", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -1, EndsInDays: 2, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 0, CI: "", Pinned: false, Paused: false, Labels: []string{"perf"}, UpdatedMinutesAgo: 1},
		{Project: "web", Number: 118, Title: "Dark mode for settings page", State: "working", Role: "Worker", Agent: "claude", Model: "claude-sonnet-4-5", Thinking: "medium", Perm: "auto-edits", Package: "", Doing: "Updating theme tokens in settings.tsx", Reason: "", ReasonKind: "", Context: 31, StartsInDays: -1, EndsInDays: 2, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 0, CI: "running", Pinned: false, Paused: false, Labels: []string{"ui"}, UpdatedMinutesAgo: 1},
		{Project: "web", Number: 119, Title: "Migrate tables to TanStack Table v8", State: "needs", Role: "Worker", Agent: "codex", Model: "gpt-5-codex", Thinking: "high", Perm: "auto-edits", Package: "", Doing: "", Reason: "Stuck: same type error 3 times in columns.tsx", ReasonKind: "stuck", Context: 81, StartsInDays: -5, EndsInDays: 1, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 0, CI: "failed", Pinned: false, Paused: false, Labels: []string{"refactor"}, UpdatedMinutesAgo: 25},
		{Project: "web", Number: 115, Title: "Chart tooltips cut off on small screens", State: "review", Role: "Worker", Agent: "gemini", Model: "gemini-2.5-pro", Thinking: "low", Perm: "auto-edits", Package: "", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -4, EndsInDays: -1, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 902, CI: "passed", Pinned: false, Paused: false, Labels: []string{"bug", "ui"}, UpdatedMinutesAgo: 180},
		{Project: "web", Number: 116, Title: "Add CSV export to reports", State: "ready", Role: "Worker", Agent: "claude", Model: "claude-sonnet-4-5", Thinking: "medium", Perm: "auto-edits", Package: "", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -6, EndsInDays: -1, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 899, CI: "passed", Pinned: false, Paused: false, Labels: []string{"feature"}, UpdatedMinutesAgo: 18},
		{Project: "web", Number: 110, Title: "Fix flaky login e2e test", State: "done", Role: "Tester", Agent: "codex", Model: "gpt-5-codex", Thinking: "medium", Perm: "auto-edits", Package: "", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -10, EndsInDays: -8, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 891, CI: "passed", Pinned: false, Paused: false, Labels: []string{"bug"}, UpdatedMinutesAgo: 1560},
		{Project: "web", Number: 111, Title: "Update onboarding copy", State: "done", Role: "Docs writer", Agent: "builtin", Model: "claude-haiku-4-5", Thinking: "low", Perm: "auto-edits", Package: "", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -9, EndsInDays: -8, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 893, CI: "passed", Pinned: false, Paused: false, Labels: []string{"docs"}, UpdatedMinutesAgo: 2400},
		{Project: "web", Number: 120, Title: "Keyboard shortcuts help dialog", State: "backlog", Role: "Worker", Agent: "claude", Model: "claude-sonnet-4-5", Thinking: "medium", Perm: "auto-edits", Package: "", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: 4, EndsInDays: 6, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: true, PullNumber: 0, CI: "", Pinned: false, Paused: false, Labels: []string{"ui"}, UpdatedMinutesAgo: 300},
		{Project: "web", Number: 121, Title: "Virtualize activity table", State: "backlog", Role: "Worker", Agent: "claude", Model: "claude-sonnet-4-5", Thinking: "medium", Perm: "auto-edits", Package: "", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: 2, EndsInDays: 5, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 0, CI: "", Pinned: false, Paused: false, Labels: []string{"perf"}, UpdatedMinutesAgo: 360},
		{Project: "web", Number: 117, Title: "Session timeout warning banner", State: "planning", Role: "UI checker", Agent: "claude", Model: "claude-haiku-4-5", Thinking: "low", Perm: "plan", Package: "", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: 1, EndsInDays: 3, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 0, CI: "", Pinned: false, Paused: false, Labels: []string{"ui"}, UpdatedMinutesAgo: 120},
		{Project: "mobile", Number: 209, Title: "Biometric login on Android", State: "working", Role: "Worker", Agent: "claude", Model: "claude-opus-4-1", Thinking: "high", Perm: "bypass", Package: "apps/android", Doing: "Running ./gradlew :app:testDebugUnitTest", Reason: "", ReasonKind: "", Context: 58, StartsInDays: -3, EndsInDays: 2, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 0, CI: "running", Pinned: false, Paused: false, Labels: []string{"auth"}, UpdatedMinutesAgo: 1},
		{Project: "mobile", Number: 207, Title: "Shared Button component variants", State: "review", Role: "Worker", Agent: "gemini", Model: "gemini-2.5-pro", Thinking: "medium", Perm: "auto-edits", Package: "packages/ui", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -5, EndsInDays: -1, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 1432, CI: "passed", Pinned: true, Paused: false, Labels: []string{"ui"}, UpdatedMinutesAgo: 44},
		{Project: "mobile", Number: 210, Title: "Offline queue for API client", State: "needs", Role: "Integrator", Agent: "claude", Model: "claude-opus-4-1", Thinking: "high", Perm: "full-auto", Package: "packages/api-client", Doing: "", Reason: "Merge conflict: queue.ts also changed by #208", ReasonKind: "conflict", Context: 22, StartsInDays: -4, EndsInDays: 1, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 1435, CI: "passed", Pinned: false, Paused: false, Labels: []string{"feature"}, UpdatedMinutesAgo: 20},
		{Project: "mobile", Number: 208, Title: "Typed errors in api-client", State: "ready", Role: "Worker", Agent: "codex", Model: "gpt-5-codex", Thinking: "medium", Perm: "auto-edits", Package: "packages/api-client", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -7, EndsInDays: -2, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 1429, CI: "passed", Pinned: false, Paused: false, Labels: nil, UpdatedMinutesAgo: 240},
		{Project: "mobile", Number: 205, Title: "Refresh tokens in secure storage", State: "done", Role: "Worker", Agent: "claude", Model: "claude-sonnet-4-5", Thinking: "medium", Perm: "auto-edits", Package: "packages/auth", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -11, EndsInDays: -7, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 1420, CI: "passed", Pinned: false, Paused: false, Labels: []string{"auth"}, UpdatedMinutesAgo: 4320},
		{Project: "mobile", Number: 211, Title: "Push notification deep links", State: "backlog", Role: "Worker", Agent: "claude", Model: "claude-sonnet-4-5", Thinking: "medium", Perm: "auto-edits", Package: "apps/ios", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: 5, EndsInDays: 9, HasStart: true, HasEnd: true, DueInDays: 9, HasDue: true, PullNumber: 0, CI: "", Pinned: false, Paused: false, Labels: []string{"feature"}, UpdatedMinutesAgo: 480},
		{Project: "mobile", Number: 212, Title: "Upgrade React Native to 0.76", State: "backlog", Role: "Worker", Agent: "codex", Model: "gpt-5-codex", Thinking: "medium", Perm: "auto-edits", Package: "apps/ios", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: 6, EndsInDays: 11, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 0, CI: "", Pinned: false, Paused: false, Labels: []string{"deps"}, UpdatedMinutesAgo: 540},
		{Project: "mobile", Number: 206, Title: "iOS splash screen flicker", State: "review", Role: "Worker", Agent: "codex", Model: "gpt-5-codex", Thinking: "low", Perm: "auto-edits", Package: "apps/ios", Doing: "", Reason: "", ReasonKind: "", Context: 22, StartsInDays: -3, EndsInDays: 0, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 1431, CI: "passed", Pinned: false, Paused: false, Labels: []string{"bug"}, UpdatedMinutesAgo: 300},
		{Project: "mobile", Number: 213, Title: "Fix Android e2e failing on main", State: "working", Role: "Tester", Agent: "codex", Model: "gpt-5-codex", Thinking: "medium", Perm: "full-auto", Package: "apps/android", Doing: "Reading the failed step log from the android workflow", Reason: "", ReasonKind: "", Context: 22, StartsInDays: 0, EndsInDays: 1, HasStart: true, HasEnd: true, DueInDays: 0, HasDue: false, PullNumber: 0, CI: "failed", Pinned: false, Paused: false, Labels: []string{"ci"}, UpdatedMinutesAgo: 1}}
}

// loadCards writes the prototype's cards for a project, and says how many it made. It is safe to
// call on every start: a project that already has its cards is left alone.
func (l *loader) loadCards(ctx context.Context, projectID string) (int, error) {
	wanted := 0
	for _, card := range prototypeCards() {
		if card.Project == projectID {
			wanted++
		}
	}
	if wanted == 0 {
		return 0, nil
	}
	existing, err := l.cardCount(ctx, projectID)
	if err != nil {
		return 0, err
	}
	if existing >= wanted {
		l.log.Info("fixture cards already there, leaving them as they are",
			"project_id", projectID, "cards", existing)
		return 0, nil
	}
	labels, err := l.ensureLabels(ctx, projectID)
	if err != nil {
		return 0, err
	}
	now := l.now()
	made := 0
	for _, card := range prototypeCards() {
		if card.Project != projectID {
			continue
		}
		created, err := l.projects.CreateCard(ctx, projectID, cardInputOf(card), projects.WithFields(cardFieldsOf(card, labels, now)))
		if err != nil {
			return made, fmt.Errorf("add card %s#%d: %w", card.Project, card.Number, err)
		}
		made++
		if state, ok := fixtureSessionState(card.State); ok && l.sessions != nil {
			if err := l.sessions.SeedSession(ctx, created.ID, state); err != nil {
				return made, fmt.Errorf("seed the session of %s#%d: %w", card.Project, card.Number, err)
			}
		}
	}
	l.log.Info("fixture cards created", "project_id", projectID, "cards", made)
	return made, nil
}

// cardCount is how many cards a project already has.
func (l *loader) cardCount(ctx context.Context, projectID string) (int, error) {
	snapshot, err := l.projects.Board(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("read the board of %s: %w", projectID, err)
	}
	return len(snapshot.Cards), nil
}

// ensureLabels makes the labels the prototype's cards carry, keyed by name. A label that is there
// already is reused, so this is safe to run again.
func (l *loader) ensureLabels(ctx context.Context, projectID string) (map[string]string, error) {
	snapshot, err := l.projects.Labels(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("read the labels of %s: %w", projectID, err)
	}
	byName := make(map[string]string, len(snapshot.Labels))
	for _, label := range snapshot.Labels {
		byName[label.Name] = label.ID
	}
	next := 0
	for _, card := range prototypeCards() {
		if card.Project != projectID {
			continue
		}
		for _, name := range card.Labels {
			if _, ok := byName[name]; ok {
				continue
			}
			label, err := l.projects.CreateLabel(ctx, projectID, protocol.CreateLabelRequest{
				Name: name, Color: labelColorFor(name, next),
			})
			if err != nil {
				return nil, fmt.Errorf("add the label %q to %s: %w", name, projectID, err)
			}
			byName[name] = label.ID
			next++
		}
	}
	return byName, nil
}

// fixtureSessionState is the session state a card's own state is drawn with: a card whose agent is
// on a turn shows as working, one that has started and is waiting shows as awake, and a card in the
// backlog or done has no session at all (it was never started, or it is merged and closed).
//
// No process is started behind such a session: see fixture.Sessions.
func fixtureSessionState(state protocol.CardState) (protocol.SessionState, bool) {
	switch state {
	case protocol.CardStateWorking, protocol.CardStatePlanning, protocol.CardStateMerging:
		return protocol.SessionStateWorking, true
	case protocol.CardStateNeeds, protocol.CardStateReview, protocol.CardStateReady:
		return protocol.SessionStateAwake, true
	default:
		return "", false
	}
}

// labelColorFor gives each label of a project a color from the fixed set, in a fixed order, so the
// same fixture always looks the same. The design has no label colors yet (Q20), so this only
// decides what the daemon stores.
func labelColorFor(_ string, index int) protocol.LabelColor {
	colors := protocol.LabelColorValues()
	return colors[index%len(colors)]
}

// cardInputOf is the part of a card a request could carry.
func cardInputOf(card prototypeCard) projects.CardInput {
	return projects.CardInput{
		Title:          card.Title,
		Agent:          card.Agent,
		Model:          card.Model,
		Thinking:       card.Thinking,
		PermissionMode: card.Perm,
		Role:           card.Role,
		Package:        card.Package,
	}
}

// cardFieldsOf is the rest: the number, the state, the times, and the fields a person's create
// request does not carry.
func cardFieldsOf(card prototypeCard, labels map[string]string, now time.Time) projects.CardFields {
	updated := now.Add(-time.Duration(card.UpdatedMinutesAgo) * time.Minute)
	fields := projects.CardFields{
		Number:      card.Number,
		State:       card.State,
		CreatedAt:   updated,
		UpdatedAt:   updated,
		DoingNow:    card.Doing,
		ContextUsed: card.Context,
		Pinned:      card.Pinned,
		Paused:      card.Paused,
		Labels:      labelIDsOf(card.Labels, labels),
	}
	if card.HasStart || card.HasEnd {
		// The prototype's board counts days from the day it loads.
		if card.HasStart {
			fields.PlannedStart = dayFrom(now, card.StartsInDays)
		}
		if card.HasEnd {
			fields.PlannedEnd = dayFrom(now, card.EndsInDays)
		}
	}
	if card.HasDue {
		fields.Due = dayFrom(now, card.DueInDays)
	}
	if card.PullNumber > 0 {
		fields.PullRequest = &protocol.PullRequest{Number: card.PullNumber}
	}
	if card.CI != "" {
		ci := card.CI
		fields.CI = &ci
	}
	if card.ReasonKind != "" {
		fields.Needs = &protocol.NeedsReason{Kind: card.ReasonKind, Text: card.Reason}
		fields.NeedsSince = updated
	}
	return fields
}

// labelIDsOf turns the prototype's label names into the project's label ids.
func labelIDsOf(names []string, labels map[string]string) []string {
	ids := make([]string, 0, len(names))
	for _, name := range names {
		if id, ok := labels[name]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// dayFrom is the start of the day this many days from now, at the given hour, so the Timeline and
// the calendar draw a span rather than a moment.
func dayFrom(now time.Time, days int) time.Time {
	day := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
	return day.AddDate(0, 0, days)
}
