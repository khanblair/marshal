package briefs

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// CISource reads every project's CI health. The daemon's is the CI service.
type CISource interface {
	Snapshot(ctx context.Context) (protocol.CISnapshot, error)
}

// SetCI gives briefs the CI health for their main-branch section. Without it that section is left out.
func (s *Service) SetCI(ci CISource) { s.ci = ci }

// Brief is a composed brief: its title, its text, and whether it had anything to report.
type Brief struct {
	Title string
	// Body is the text under the title, as every channel gets it.
	Body string
	// Empty is true when no section found anything to say, so a quiet schedule can stay quiet.
	Empty bool
}

// maxListed is how many cards one section lists before it says how many more there are.
const maxListed = 8

// staleAfter is how long a card must sit still to be called stale.
const staleAfter = 7 * 24 * time.Hour

// weekAhead is how far the weekly review's calendar reaches.
const weekAhead = 7

// projectCards is one project and its cards, read once so every section sees the same board.
type projectCards struct {
	project protocol.Project
	cards   []protocol.Card
}

// part is what one section wrote: its text, and how many things it found to report.
type part struct {
	text  string
	items int
}

// material is everything a brief reads, gathered once.
type material struct {
	now      time.Time
	since    time.Time
	projects []projectCards
}

// Build composes a brief from the parts the schedule lists, in the order it lists them.
func (s *Service) Build(ctx context.Context, sched protocol.Schedule, since time.Time) (Brief, error) {
	now := s.now()
	since = boundedSince(since, now, sched.Template)
	data, err := s.read(ctx, now, since)
	if err != nil {
		return Brief{}, err
	}
	var parts []string
	items := 0
	for _, id := range sched.Sections {
		got := s.section(ctx, id, sched, data)
		if got.text == "" {
			continue
		}
		parts = append(parts, got.text)
		items += got.items
	}
	title := fmt.Sprintf("%s - %s", sched.Name, now.Format("Mon Jan 2"))
	if len(parts) == 0 {
		return Brief{Title: title, Body: "Nothing to report.", Empty: true}, nil
	}
	return Brief{Title: title, Body: strings.Join(parts, "\n\n"), Empty: items == 0}, nil
}

// boundedSince keeps a brief from reading further back than its template allows, so the first one
// of a schedule made weeks ago does not report on all of them.
func boundedSince(since, now time.Time, template string) time.Time {
	earliest := now.Add(-Lookback(template))
	if since.Before(earliest) {
		return earliest
	}
	return since
}

func (s *Service) read(ctx context.Context, now, since time.Time) (material, error) {
	list, err := s.projects.List(ctx)
	if err != nil {
		return material{}, fmt.Errorf("list the projects for the brief: %w", err)
	}
	data := material{now: now, since: since}
	for _, project := range list.Projects {
		cards, err := s.projects.Cards(ctx, project.ID)
		if err != nil {
			return material{}, fmt.Errorf("read %s's cards for the brief: %w", project.Name, err)
		}
		data.projects = append(data.projects, projectCards{project: project, cards: cards})
	}
	return data, nil
}

// section writes one part. An id it does not know writes nothing.
func (s *Service) section(ctx context.Context, id string, sched protocol.Schedule, data material) part {
	switch id {
	case SectionCalendar:
		return s.calendarPart(ctx, sched, data.now)
	case SectionNeedsYou:
		return needsYouPart(data)
	case SectionWorking:
		return cardsPart("Still working", "Nothing is running.", pick(data, isWorking), data, nil)
	case SectionInReview:
		return cardsPart("In review", "Nothing is in review.", pick(data, inReview), data, reviewNote)
	case SectionFinished:
		return cardsPart("Finished", "Nothing finished.", pick(data, func(c protocol.Card, d material) bool {
			return c.State == protocol.CardStateDone && c.UpdatedAt.Time().After(d.since)
		}), data, nil)
	case SectionCardCI:
		return cardsPart("Card CI failures", "No card's CI failed.", pick(data, func(c protocol.Card, d material) bool {
			return c.CI != nil && *c.CI == protocol.CIStateFailed && c.UpdatedAt.Time().After(d.since)
		}), data, nil)
	case SectionStale:
		return cardsPart("Stale cards", "No card has sat still for a week.", pick(data, isStale), data, staleNote)
	case SectionMainCI:
		return s.mainCIPart(ctx, data)
	}
	return part{}
}

// pick gathers the cards, across projects, that match, longest-untouched first.
func pick(data material, match func(protocol.Card, material) bool) []protocol.Card {
	var out []protocol.Card
	for _, project := range data.projects {
		for _, card := range project.cards {
			if match(card, data) {
				out = append(out, card)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.Time().Before(out[j].UpdatedAt.Time()) })
	return out
}

func isWorking(c protocol.Card, _ material) bool {
	return c.State == protocol.CardStateWorking && !c.Paused
}

func inReview(c protocol.Card, _ material) bool { return c.State == protocol.CardStateReview }

func isStale(c protocol.Card, d material) bool {
	switch c.State {
	case protocol.CardStateBacklog, protocol.CardStateDone:
		return false
	}
	return c.UpdatedAt.Time().Before(d.now.Add(-staleAfter))
}

// reviewNote is a review card's pull request.
func reviewNote(c protocol.Card, _ material) string {
	if c.PullRequest == nil {
		return "no pull request yet"
	}
	return fmt.Sprintf("PR #%d", c.PullRequest.Number)
}

// staleNote says how long a card has sat and where it sits.
func staleNote(c protocol.Card, d material) string {
	return fmt.Sprintf("%s, last moved %s ago", c.State, ago(d.now.Sub(c.UpdatedAt.Time())))
}

// needsYouPart lists the cards waiting on a person, with how long and why.
func needsYouPart(data material) part {
	cards := pick(data, func(c protocol.Card, _ material) bool { return c.State == protocol.CardStateNeeds })
	return cardsPart("Needs you", "Nothing is waiting on you.", cards, data, func(c protocol.Card, d material) string {
		note := "waiting " + ago(d.now.Sub(c.UpdatedAt.Time()))
		if c.NeedsReason != nil && strings.TrimSpace(c.NeedsReason.Text) != "" {
			note += ": " + strings.TrimSpace(c.NeedsReason.Text)
		}
		return note
	})
}

// cardsPart writes a titled list of cards, or the sentence for an empty one.
func cardsPart(title, none string, cards []protocol.Card, data material, note func(protocol.Card, material) string) part {
	if len(cards) == 0 {
		return part{text: "## " + title + "\n" + none}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s (%d)\n", title, len(cards))
	for i, card := range cards {
		if i == maxListed {
			fmt.Fprintf(&b, "- ... and %d more\n", len(cards)-maxListed)
			break
		}
		line := fmt.Sprintf("- %s %s", card.Key, card.Title)
		if note != nil {
			if text := note(card, data); text != "" {
				line += " (" + text + ")"
			}
		}
		b.WriteString(line + "\n")
	}
	return part{text: strings.TrimRight(b.String(), "\n"), items: len(cards)}
}

// mainCIPart says whether each project's default branch is passing. A project's health is the newest
// run of each workflow on its default branch, so one failing workflow is not hidden by another that
// passed later, and a failing card branch is not mistaken for main. A project with no run on its
// default branch says so, and with no run anywhere the part says no run has been reported.
func (s *Service) mainCIPart(ctx context.Context, data material) part {
	if s.ci == nil {
		return part{}
	}
	snapshot, err := s.ci.Snapshot(ctx)
	if err != nil {
		return part{text: "## Main branch CI\nCI health could not be read."}
	}
	if len(snapshot.Projects) == 0 {
		return part{text: "## Main branch CI\nNo CI runs have been reported yet."}
	}
	names, branches := map[string]string{}, map[string]string{}
	for _, project := range data.projects {
		names[project.project.ID] = project.project.Name
		branches[project.project.ID] = project.project.DefaultBranch
	}
	var b strings.Builder
	b.WriteString("## Main branch CI\n")
	failing := 0
	for _, entry := range snapshot.Projects {
		name := names[entry.ProjectID]
		if name == "" {
			name = entry.ProjectID
		}
		line, failed := mainLine(name, entry, branches[entry.ProjectID])
		if failed {
			failing++
		}
		b.WriteString(line + "\n")
	}
	return part{text: strings.TrimRight(b.String(), "\n"), items: failing}
}

// mainLine is one project's line, and whether its default branch is failing.
func mainLine(name string, entry protocol.ProjectCI, branch string) (string, bool) {
	if branch == "" {
		branch = "main"
	}
	seen := map[string]bool{}
	var failing []protocol.CiRun
	running, found := false, false
	for _, run := range entry.Runs {
		// The runs arrive newest first, so the first of a workflow on the branch is the one that counts.
		if run.Branch != branch || seen[run.Workflow] {
			continue
		}
		seen[run.Workflow] = true
		found = true
		switch run.Status {
		case protocol.CIStateFailed:
			failing = append(failing, run)
		case protocol.CIStateRunning, protocol.CIStateQueued:
			running = true
		}
	}
	switch {
	case len(failing) > 0:
		workflows := make([]string, len(failing))
		for i, run := range failing {
			workflows[i] = run.Workflow
		}
		line := fmt.Sprintf("- %s: failing (%s)", name, strings.Join(workflows, ", "))
		if failing[0].URL != "" {
			line += " " + failing[0].URL
		}
		return line, true
	case running:
		return fmt.Sprintf("- %s: running", name), false
	case found:
		return fmt.Sprintf("- %s: passing", name), false
	}
	return fmt.Sprintf("- %s: no run on %s yet", name, branch), false
}

// ago writes how long ago in the coarsest unit that says it.
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
