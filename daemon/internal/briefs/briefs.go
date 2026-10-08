// Package briefs composes the morning and evening briefs (docs/marshal-product-scope.md section
// 18, docs/backend-checklist.md B8.5, build-plan task 8.7): what waits on a person, what is in
// review, what finished, and what failed, gathered across every project and grouped by project
// (18.1's "across projects" rule). It reads only the projects service every card route already
// reads - never a query of its own - and writes a deterministic list. The short model-written
// summary 18.5 also asks for, and delivering a brief anywhere but as the text this package
// returns (chat, Telegram, Discord, email, an Obsidian daily note), are both later polish: see
// docs/marshal-product-scope.md, section 18.
package briefs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations/googlecal"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/runctx"
	"github.com/khanblair/marshal/daemon/internal/zone"
)

// EventSource reads Google Calendar's events. The daemon's is the integrations service. A brief
// uses it for the calendar part of its text, and leaves that part out when Google is not connected.
type EventSource interface {
	GoogleEvents(ctx context.Context, start, end time.Time) ([]googlecal.Event, protocol.GoogleReading)
}

// Projects is what a brief reads of the board: the projects and each one's cards. The daemon's is the
// projects service every card route reads.
type Projects interface {
	List(ctx context.Context) (protocol.ProjectListSnapshot, error)
	Cards(ctx context.Context, projectID string) ([]protocol.Card, error)
}

// Service composes a brief.
type Service struct {
	projects Projects
	events   EventSource
	ci       CISource
	deliver  Deliverer
	now      func() time.Time
}

// New builds the service. projects is required.
func New(projects Projects) *Service {
	return &Service{projects: projects, now: time.Now}
}

// SetEvents gives briefs Google Calendar's events, for their calendar section. Until it is called,
// a brief has no calendar section. The daemon calls it once, while it starts.
func (s *Service) SetEvents(events EventSource) { s.events = events }

// SetClock sets what a brief takes "today" and "tomorrow" from. Nil is ignored.
func (s *Service) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// Handle is a schedules.ActionHandler for a brief: compose it, send it to the channels the schedule
// names, and return it as the run's own details, so a person can read what a brief said through the
// schedule's run history (ListScheduleRuns), whether or not a chat got it. sched.Name carries the
// brief's own title ("Morning brief", "Evening wind-down" - whatever a person named it), and heads the
// text.
//
// A schedule with no sections is a brief made the way briefs were before sections: cards grouped by
// project, and the calendar. It is kept for the schedules that already exist.
func (s *Service) Handle(ctx context.Context, sched protocol.Schedule, since time.Time) (string, error) {
	if len(sched.Sections) == 0 {
		return s.handleLegacy(ctx, sched, since)
	}
	brief, err := s.Build(ctx, sched, since)
	if err != nil {
		return "", err
	}
	if due, late := runctx.Late(ctx); late {
		brief.Body = lateLine(due, s.now()) + "\n\n" + brief.Body
	}
	text := fmt.Sprintf("# %s\n\n%s", brief.Title, brief.Body)
	if sched.QuietWhenEmpty && brief.Empty {
		return text + "\n\n---\nNothing to report, so nothing was sent.", nil
	}
	lines, failed := s.send(ctx, sched, brief)
	if len(lines) > 0 {
		text += "\n\n---\n" + strings.Join(lines, "\n")
	}
	if failed {
		return text, errDelivery
	}
	return text, nil
}

// lateLine says a brief is a catch-up: when it was due, and when it is being sent. It names the zone
// so the two times mean one thing wherever the message is read.
func lateLine(due, now time.Time) string {
	return fmt.Sprintf("Sent late: due %s, sent %s (%s) because Marshal was not running then.",
		due.In(now.Location()).Format("15:04 Mon"), now.Format("15:04"), zone.Label(now))
}

func (s *Service) handleLegacy(ctx context.Context, sched protocol.Schedule, since time.Time) (string, error) {
	content, err := s.Compose(ctx, boundedSince(since, s.now(), sched.Template))
	if err != nil {
		return "", err
	}
	if calendar := s.calendarSection(ctx, sched); calendar != "" {
		content = calendar + "\n\n" + content
	}
	return fmt.Sprintf("# %s\n\n%s", sched.Name, content), nil
}

// projectBrief is what changed in one project since the brief before.
type projectBrief struct {
	name      string
	needsYou  []protocol.Card
	inReview  []protocol.Card
	doneSince []protocol.Card
	ciFailed  []protocol.Card
}

func (b projectBrief) empty() bool {
	return len(b.needsYou) == 0 && len(b.inReview) == 0 && len(b.doneSince) == 0 && len(b.ciFailed) == 0
}

// Compose gathers, across every project, the cards that wait on a person, the cards in review, the
// cards that finished since `since`, and the cards whose CI failed since `since`, and writes them
// as one brief, grouped by project. A project with nothing to report is left out rather than
// printed empty.
func (s *Service) Compose(ctx context.Context, since time.Time) (string, error) {
	projectList, err := s.projects.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list the projects for the brief: %w", err)
	}
	var reports []projectBrief
	for _, project := range projectList.Projects {
		cards, err := s.projects.Cards(ctx, project.ID)
		if err != nil {
			return "", fmt.Errorf("read %s's cards for the brief: %w", project.Name, err)
		}
		report := gatherProject(project.Name, cards, since)
		if !report.empty() {
			reports = append(reports, report)
		}
	}
	return render(reports, since), nil
}

// gatherProject sorts name's cards into the brief's four groups. A card can be in at most one of
// needsYou/inReview/doneSince (they follow the board's own mutually exclusive states) and,
// independently, in ciFailed too, since a card's CI state is not its board state.
func gatherProject(name string, cards []protocol.Card, since time.Time) projectBrief {
	report := projectBrief{name: name}
	for _, card := range cards {
		switch {
		case card.State == protocol.CardStateNeeds:
			report.needsYou = append(report.needsYou, card)
		case card.State == protocol.CardStateReview:
			report.inReview = append(report.inReview, card)
		case card.State == protocol.CardStateDone && card.UpdatedAt.Time().After(since):
			report.doneSince = append(report.doneSince, card)
		}
		if card.CI != nil && *card.CI == protocol.CIStateFailed && card.UpdatedAt.Time().After(since) {
			report.ciFailed = append(report.ciFailed, card)
		}
	}
	return report
}

// render writes every project's report as markdown headings and lists. "Nothing new" is its own
// sentence rather than an empty document, so a person reading a run's details is never left
// guessing whether the brief ran at all.
func render(reports []projectBrief, since time.Time) string {
	if len(reports) == 0 {
		return fmt.Sprintf("Nothing new since %s.", since.Format("Jan 2 15:04"))
	}
	var b strings.Builder
	for _, report := range reports {
		fmt.Fprintf(&b, "## %s\n", report.name)
		writeCardList(&b, "Needs you", report.needsYou)
		writeCardList(&b, "In review", report.inReview)
		writeCardList(&b, "Done", report.doneSince)
		writeCardList(&b, "CI failed", report.ciFailed)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// writeCardList appends label's section when cards holds anything, one line per card as its key
// and title - the same pair `docs/architecture.md`'s own card-key convention uses everywhere else.
func writeCardList(b *strings.Builder, label string, cards []protocol.Card) {
	if len(cards) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", label)
	for _, card := range cards {
		fmt.Fprintf(b, "- %s %s\n", card.Key, card.Title)
	}
}
