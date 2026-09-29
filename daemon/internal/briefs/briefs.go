// Package briefs composes the morning and evening briefs (docs/marshal-product-scope.md section
// 18, docs/backend-checklist.md B8.5, build-plan task 8.7): what waits on a person, what is in
// review, what finished, and what failed, gathered across every project and grouped by project
// (18.1's "across projects" rule). It reads only the projects service every card route already
// reads - never a query of its own - and writes a deterministic list. The short model-written
// summary 18.5 also asks for, and delivering a brief anywhere but as the text this package
// returns (chat, Telegram, Discord, email, an Obsidian daily note), are both later polish: see
// phase-reports/phase-08-automation.md.
package briefs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Service composes a brief.
type Service struct {
	projects *projects.Service
}

// New builds the service. projects is required.
func New(projects *projects.Service) *Service {
	return &Service{projects: projects}
}

// Handle is a schedules.ActionHandler for a brief: compose it and return it as the run's own
// details, so a person can read what a brief said through the schedule's run history
// (ListScheduleRuns) even before any delivery channel exists. sched.Name carries the brief's own
// title ("Morning brief", "Evening brief" - whatever a person named it), and heads the text.
func (s *Service) Handle(ctx context.Context, sched protocol.Schedule, since time.Time) (string, error) {
	content, err := s.Compose(ctx, since)
	if err != nil {
		return "", err
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
