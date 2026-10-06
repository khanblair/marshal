package briefs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations/googlecal"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

type fakeProjects struct {
	projects []protocol.Project
	cards    map[string][]protocol.Card
}

func (f fakeProjects) List(context.Context) (protocol.ProjectListSnapshot, error) {
	return protocol.ProjectListSnapshot{Projects: f.projects}, nil
}

func (f fakeProjects) Cards(_ context.Context, id string) ([]protocol.Card, error) {
	return f.cards[id], nil
}

type fakeCI struct{ snapshot protocol.CISnapshot }

func (f fakeCI) Snapshot(context.Context) (protocol.CISnapshot, error) { return f.snapshot, nil }

type sent struct{ channel, title, body string }

type fakeDeliverer struct {
	calls []sent
	err   map[string]error
}

func (f *fakeDeliverer) Deliver(_ context.Context, channel, title, body string) error {
	f.calls = append(f.calls, sent{channel, title, body})
	return f.err[channel]
}

func card(key, title string, state protocol.CardState, updated time.Time) protocol.Card {
	return protocol.Card{Key: key, Title: title, State: state, UpdatedAt: protocol.NewTimestamp(updated)}
}

func board(cards ...protocol.Card) fakeProjects {
	return fakeProjects{
		projects: []protocol.Project{{ID: "api", Name: "api", DefaultBranch: "main"}},
		cards:    map[string][]protocol.Card{"api": cards},
	}
}

func briefService(projects Projects) *Service {
	s := New(projects)
	s.SetClock(func() time.Time { return noon })
	return s
}

func schedule(template string, sections ...string) protocol.Schedule {
	return protocol.Schedule{Name: "Test brief", Template: template, Sections: sections}
}

func TestABriefWritesOnlyItsSectionsAndInTheOrderGiven(t *testing.T) {
	s := briefService(board(
		card("api#1", "Fix login", protocol.CardStateNeeds, noon.Add(-3*time.Hour)),
		card("api#2", "Add cache", protocol.CardStateDone, noon.Add(-time.Hour)),
		card("api#3", "Old thing", protocol.CardStateReview, noon.Add(-time.Hour)),
	))
	got, err := s.Build(context.Background(), schedule("", SectionFinished, SectionNeedsYou), noon.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	finished, needs := strings.Index(got.Body, "## Finished (1)"), strings.Index(got.Body, "## Needs you (1)")
	if finished < 0 || needs < 0 || finished > needs {
		t.Fatalf("the sections are missing or out of order:\n%s", got.Body)
	}
	if strings.Contains(got.Body, "In review") {
		t.Fatalf("a section that was not asked for was written:\n%s", got.Body)
	}
	if got.Empty {
		t.Fatal("a brief with cards in it was called empty")
	}
}

func TestNeedsYouSaysHowLongACardHasWaitedAndWhy(t *testing.T) {
	waiting := card("api#1", "Fix login", protocol.CardStateNeeds, noon.Add(-3*time.Hour))
	waiting.NeedsReason = &protocol.NeedsReason{Text: "Approve the migration"}
	s := briefService(board(waiting))
	got, _ := s.Build(context.Background(), schedule("", SectionNeedsYou), noon.Add(-24*time.Hour))
	if !strings.Contains(got.Body, "- api#1 Fix login (waiting 3h: Approve the migration)") {
		t.Fatalf("the waiting line is wrong:\n%s", got.Body)
	}
}

func TestABriefWithNothingToSayIsEmptyAndSaysSo(t *testing.T) {
	s := briefService(board())
	got, _ := s.Build(context.Background(), schedule("", SectionNeedsYou, SectionWorking), noon.Add(-24*time.Hour))
	if !got.Empty || !strings.Contains(got.Body, "Nothing is waiting on you.") {
		t.Fatalf("an empty brief is %+v", got)
	}
}

func TestFinishedCardsOlderThanTheLookbackAreLeftOut(t *testing.T) {
	old := card("api#1", "Ancient", protocol.CardStateDone, noon.Add(-30*24*time.Hour))
	recent := card("api#2", "Recent", protocol.CardStateDone, noon.Add(-2*time.Hour))
	s := briefService(board(old, recent))
	// A schedule made a month ago has never run: its "since" is its creation date.
	got, _ := s.Build(context.Background(), schedule(TemplateMorning, SectionFinished), noon.Add(-30*24*time.Hour))
	if strings.Contains(got.Body, "Ancient") || !strings.Contains(got.Body, "Recent") {
		t.Fatalf("the first brief of an old schedule read too far back:\n%s", got.Body)
	}
}

func TestStaleListsCardsThatHaveNotMovedInAWeek(t *testing.T) {
	s := briefService(board(
		card("api#1", "Sitting", protocol.CardStateWorking, noon.Add(-9*24*time.Hour)),
		card("api#2", "Backlog item", protocol.CardStateBacklog, noon.Add(-30*24*time.Hour)),
		card("api#3", "Fresh", protocol.CardStateWorking, noon.Add(-time.Hour)),
	))
	got, _ := s.Build(context.Background(), schedule(TemplateStale, SectionStale), noon.Add(-24*time.Hour))
	if !strings.Contains(got.Body, "api#1 Sitting (working, last moved 9d ago)") ||
		strings.Contains(got.Body, "Backlog item") || strings.Contains(got.Body, "Fresh") {
		t.Fatalf("the stale list is wrong:\n%s", got.Body)
	}
}

func TestMainCINamesAFailingWorkflowEvenWhenAnotherPassedLater(t *testing.T) {
	s := briefService(board())
	s.SetCI(fakeCI{protocol.CISnapshot{Projects: []protocol.ProjectCI{{
		ProjectID: "api", Status: protocol.CIStatePassed,
		Runs: []protocol.CiRun{
			{Workflow: "analyst-daily", Branch: "main", Status: protocol.CIStatePassed},
			{Workflow: "collector-bot", Branch: "main", Status: protocol.CIStateFailed, URL: "https://example.test/run/9"},
			{Workflow: "collector-bot", Branch: "main", Status: protocol.CIStatePassed},
		},
	}}}})
	got, _ := s.Build(context.Background(), schedule("", SectionMainCI), noon.Add(-time.Hour))
	if !strings.Contains(got.Body, "- api: failing (collector-bot) https://example.test/run/9") || got.Empty {
		t.Fatalf("a failing workflow on main was hidden by a later passing one:\n%+v", got)
	}
}

func TestMainCIIgnoresACardBranchAndCountsOnlyTheNewestRunOfAWorkflow(t *testing.T) {
	s := briefService(board())
	s.SetCI(fakeCI{protocol.CISnapshot{Projects: []protocol.ProjectCI{{
		ProjectID: "api", Status: protocol.CIStateFailed,
		Runs: []protocol.CiRun{
			{Workflow: "ci", Branch: "marshal/api-1-fix", Status: protocol.CIStateFailed},
			{Workflow: "ci", Branch: "main", Status: protocol.CIStatePassed},
			{Workflow: "ci", Branch: "main", Status: protocol.CIStateFailed},
		},
	}}}})
	got, _ := s.Build(context.Background(), schedule("", SectionMainCI), noon.Add(-time.Hour))
	if !strings.Contains(got.Body, "- api: passing") || !got.Empty {
		t.Fatalf("main was reported from a card branch or an old run:\n%+v", got)
	}
}

func TestMainCISaysARunIsOnItsWayAndWhenMainHasNoRunYet(t *testing.T) {
	s := briefService(fakeProjects{
		projects: []protocol.Project{{ID: "api", Name: "api", DefaultBranch: "main"}, {ID: "web", Name: "web", DefaultBranch: "main"}},
	})
	s.SetCI(fakeCI{protocol.CISnapshot{Projects: []protocol.ProjectCI{
		{ProjectID: "api", Runs: []protocol.CiRun{{Workflow: "ci", Branch: "main", Status: protocol.CIStateRunning}}},
		{ProjectID: "web", Runs: []protocol.CiRun{{Workflow: "ci", Branch: "marshal/web-2", Status: protocol.CIStatePassed}}},
	}}})
	got, _ := s.Build(context.Background(), schedule("", SectionMainCI), noon.Add(-time.Hour))
	if !strings.Contains(got.Body, "- api: running") || !strings.Contains(got.Body, "- web: no run on main yet") {
		t.Fatalf("the lines are wrong:\n%s", got.Body)
	}
}

func TestMainCISaysWhenNoRunHasBeenReported(t *testing.T) {
	s := briefService(board())
	s.SetCI(fakeCI{})
	got, _ := s.Build(context.Background(), schedule("", SectionMainCI), noon.Add(-time.Hour))
	if !strings.Contains(got.Body, "No CI runs have been reported yet.") {
		t.Fatalf("the empty CI part is wrong:\n%s", got.Body)
	}
}

func TestAWeeklyReviewReadsTheComingWeekOfTheCalendar(t *testing.T) {
	fake := &fakeEvents{reading: protocol.GoogleReading{Connected: true}, events: []googlecal.Event{
		{Title: "Planning", StartAt: at(10, 0).AddDate(0, 0, 3), EndAt: at(11, 0).AddDate(0, 0, 3)},
	}}
	s := briefService(board())
	s.SetEvents(fake)
	got, _ := s.Build(context.Background(), schedule(TemplateWeekly, SectionCalendar), noon.Add(-24*time.Hour))
	if !fake.start.Equal(today.AddDate(0, 0, 1)) || !fake.end.Equal(today.AddDate(0, 0, 8)) {
		t.Fatalf("the weekly calendar asked for %v to %v", fake.start, fake.end)
	}
	if !strings.Contains(got.Body, "## The coming week's calendar") || !strings.Contains(got.Body, "Mon 10:00-11:00 Planning") {
		t.Fatalf("the weekly calendar part is wrong:\n%s", got.Body)
	}
}

func TestAnEveningWindDownReadsTomorrowsCalendar(t *testing.T) {
	fake := &fakeEvents{reading: protocol.GoogleReading{Connected: true}}
	s := briefService(board())
	s.SetEvents(fake)
	_, _ = s.Build(context.Background(), schedule(TemplateWindDown, SectionCalendar), noon.Add(-time.Hour))
	if !fake.start.Equal(today.AddDate(0, 0, 1)) {
		t.Fatalf("a wind-down read the calendar from %v, want tomorrow", fake.start)
	}
}

func TestAQuietBriefWithNothingToReportSendsNothing(t *testing.T) {
	d := &fakeDeliverer{}
	s := briefService(board())
	s.SetDeliverer(d)
	sched := schedule(TemplateNeedsDigest, SectionNeedsYou)
	sched.QuietWhenEmpty, sched.Deliver = true, []string{ChannelTelegram}
	details, err := s.Handle(context.Background(), sched, noon.Add(-time.Hour))
	if err != nil || len(d.calls) != 0 || !strings.Contains(details, "Nothing to report, so nothing was sent.") {
		t.Fatalf("a quiet empty brief sent %v, err %v:\n%s", d.calls, err, details)
	}
}

func TestABriefIsSentToEachChannelAndTheRunSaysWhatHappened(t *testing.T) {
	d := &fakeDeliverer{err: map[string]error{ChannelDiscord: ErrNotConnected}}
	s := briefService(board(card("api#1", "Fix login", protocol.CardStateNeeds, noon.Add(-time.Hour))))
	s.SetDeliverer(d)
	sched := schedule(TemplateMorning, SectionNeedsYou)
	sched.Deliver = []string{ChannelTelegram, ChannelDiscord}
	details, err := s.Handle(context.Background(), sched, noon.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.calls) != 2 || d.calls[0].channel != ChannelTelegram || !strings.Contains(d.calls[0].body, "Fix login") {
		t.Fatalf("the brief went to %v", d.calls)
	}
	if !strings.Contains(details, "Sent to Telegram.") || !strings.Contains(details, "Discord is not connected, so it was skipped.") {
		t.Fatalf("the run does not say what happened:\n%s", details)
	}
}

func TestAFailedSendKeepsTheBriefAndFailsTheRun(t *testing.T) {
	d := &fakeDeliverer{err: map[string]error{ChannelTelegram: errors.New("chat not found")}}
	s := briefService(board(card("api#1", "Fix login", protocol.CardStateNeeds, noon.Add(-time.Hour))))
	s.SetDeliverer(d)
	sched := schedule(TemplateMorning, SectionNeedsYou)
	sched.Deliver = []string{ChannelTelegram}
	details, err := s.Handle(context.Background(), sched, noon.Add(-time.Hour))
	if err == nil || !strings.Contains(details, "Fix login") || !strings.Contains(details, "Could not send to Telegram: chat not found") {
		t.Fatalf("a failed send lost the brief or hid the failure (err %v):\n%s", err, details)
	}
}

func TestALongBriefIsCutToWhatTheChatTakesAndSaysWhereTheRestIs(t *testing.T) {
	long := strings.Repeat("- a card line that goes on, with an em dash \u2014 and a tick \u2713\n", 400)
	title := "Morning brief - Tue Oct 6"
	for _, channel := range []string{ChannelTelegram, ChannelDiscord, ChannelNtfy} {
		b := budgetFor(channel)
		got := fit(forChat(long), b, titleRoom(title))
		if !strings.HasSuffix(got, trimmedNote) {
			t.Fatalf("%s: the cut brief does not say where the rest is", channel)
		}
		if b.characters > 0 && len([]rune(title))+2+len([]rune(got)) > b.characters {
			t.Fatalf("%s: title and body are %d characters, over %d", channel, len([]rune(title))+2+len([]rune(got)), b.characters)
		}
		if b.bytes > 0 && len(got) > b.bytes {
			t.Fatalf("%s: the body is %d bytes, over %d", channel, len(got), b.bytes)
		}
	}
	if short := fit("short", budgetFor(ChannelTelegram), titleRoom(title)); short != "short" {
		t.Fatalf("a short brief was changed to %q", short)
	}
}

func TestAChatGetsPlainHeadingsAndTheHistoryKeepsTheMarkdown(t *testing.T) {
	d := &fakeDeliverer{}
	s := briefService(board(card("api#1", "Fix login", protocol.CardStateNeeds, noon.Add(-time.Hour))))
	s.SetDeliverer(d)
	sched := schedule(TemplateMorning, SectionNeedsYou)
	sched.Deliver = []string{ChannelTelegram}
	details, err := s.Handle(context.Background(), sched, noon.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(d.calls[0].body, "## ") || !strings.Contains(d.calls[0].body, "Needs you (1)\n- api#1 Fix login") {
		t.Fatalf("the chat got %q", d.calls[0].body)
	}
	if !strings.Contains(details, "## Needs you (1)") {
		t.Fatalf("the history lost its headings:\n%s", details)
	}
}

func TestTheFirstBriefOfANewScheduleReadsBackByItsTemplateNotFromItsCreation(t *testing.T) {
	finishedYesterday := card("api#2", "Done yesterday", protocol.CardStateDone, noon.Add(-20*time.Hour))
	s := briefService(board(finishedYesterday))
	// A schedule that has never run has no since: the zero time.
	got, _ := s.Build(context.Background(), schedule(TemplateMorning, SectionFinished), time.Time{})
	if !strings.Contains(got.Body, "Done yesterday") {
		t.Fatalf("the first brief missed work finished yesterday:\n%s", got.Body)
	}
	old := card("api#3", "Done last month", protocol.CardStateDone, noon.Add(-30*24*time.Hour))
	s = briefService(board(old))
	got, _ = s.Build(context.Background(), schedule(TemplateMorning, SectionFinished), time.Time{})
	if strings.Contains(got.Body, "Done last month") {
		t.Fatalf("the first brief read back a month:\n%s", got.Body)
	}
}

func TestEveryTemplateUsesKnownSectionsAndChannelsAndStartsOff(t *testing.T) {
	seen := map[string]bool{}
	for _, tmpl := range Templates() {
		if seen[tmpl.Key] {
			t.Fatalf("two templates share the key %q", tmpl.Key)
		}
		seen[tmpl.Key] = true
		if len(tmpl.Sections) == 0 || tmpl.Name == "" || tmpl.Summary == "" {
			t.Fatalf("template %q is incomplete: %+v", tmpl.Key, tmpl)
		}
		for _, id := range tmpl.Sections {
			if !ValidSection(id) {
				t.Fatalf("template %q has an unknown section %q", tmpl.Key, id)
			}
		}
	}
	if len(seen) != 7 {
		t.Fatalf("there are %d starter templates, want the 7 the owner chose", len(seen))
	}
}

func TestAPreviewIsExactlyTheMessageTheChatGets(t *testing.T) {
	d := &fakeDeliverer{}
	s := briefService(board(card("api#1", "Fix login", protocol.CardStateNeeds, noon.Add(-time.Hour))))
	s.SetDeliverer(d)
	sched := schedule(TemplateMorning, SectionNeedsYou, SectionWorking)
	sched.Deliver = []string{ChannelTelegram}
	preview, err := s.Preview(context.Background(), sched, noon.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Handle(context.Background(), sched, noon.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	sentText := d.calls[0].title + "\n\n" + d.calls[0].body
	if preview != sentText {
		t.Fatalf("the preview differs from what was sent:\npreview: %q\nsent:    %q", preview, sentText)
	}
	if len(d.calls) != 1 {
		t.Fatalf("a preview sent something: %d messages in all", len(d.calls))
	}
}
