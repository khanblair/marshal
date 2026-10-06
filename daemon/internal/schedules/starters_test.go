package schedules_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/briefs"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/schedules"
)

func TestReconcileLetsAScheduleSWordsWinOverItsStoredTimeAndDays(t *testing.T) {
	cases := []struct {
		name     string
		req      protocol.SaveScheduleRequest
		wantTime string
		wantDays []int
	}{
		{"a new clock time replaces the old one", protocol.SaveScheduleRequest{Trigger: "Cron", When: "Every weekday at 7:30", Time: "09:00", Days: []int{1, 2, 3, 4, 5}}, "07:30", []int{1, 2, 3, 4, 5}},
		{"a day name replaces the stored days", protocol.SaveScheduleRequest{Trigger: "Cron", When: "Every Monday at 9:00", Time: "09:00", Days: []int{0}}, "09:00", []int{1}},
		{"several days read in order", protocol.SaveScheduleRequest{Trigger: "Cron", When: "Every Fri, Mon and Wed at 8:15", Time: "", Days: nil}, "08:15", []int{1, 3, 5}},
		{"weekend", protocol.SaveScheduleRequest{Trigger: "Cron", When: "Every weekend at 10:00", Days: []int{1}}, "10:00", []int{0, 6}},
		{"every day means no day list", protocol.SaveScheduleRequest{Trigger: "Cron", When: "Every day at 6:00", Days: []int{1}}, "06:00", []int{}},
		{"words with no days leave the stored days", protocol.SaveScheduleRequest{Trigger: "Cron", When: "At 6:00", Days: []int{2}}, "06:00", []int{2}},
		{"an interval is left alone", protocol.SaveScheduleRequest{Trigger: "Interval", When: "Every 4 hours", Time: "09:00", Days: []int{1}}, "09:00", []int{1}},
		{"an event is left alone", protocol.SaveScheduleRequest{Trigger: "Event", When: "When 30 minutes before my first calendar event", Time: "", Days: []int{}}, "", []int{}},
		{"an unreadable clock leaves the stored time", protocol.SaveScheduleRequest{Trigger: "Cron", When: "Every weekday at 25:00", Time: "09:00", Days: []int{1}}, "09:00", []int{1, 2, 3, 4, 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotTime, gotDays := schedules.Reconcile(tc.req)
			if gotTime != tc.wantTime || !slices.Equal(gotDays, tc.wantDays) {
				t.Fatalf("got %q %v, want %q %v", gotTime, gotDays, tc.wantTime, tc.wantDays)
			}
		})
	}
}

func TestEditingTheWordsMovesTheCronToo(t *testing.T) {
	st := newTestStore(t)
	svc := schedules.NewService(st, nil)
	ctx := context.Background()
	made, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		Name: "Brief", Kind: "brief", Trigger: "Cron", When: "Every weekday at 9:00", Time: "09:00", Days: []int{1, 2, 3, 4, 5}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The old editor sent the new words with the old time and days.
	saved, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		ID: made.ID, Name: "Brief", Kind: "brief", Trigger: "Cron", When: "Every Monday at 7:30", Time: "09:00", Days: []int{1, 2, 3, 4, 5}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	row, err := st.Queries().GetSchedule(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.CronExpr != "30 7 * * 1" || saved.Time != "07:30" || !slices.Equal(saved.Days, []int{1}) {
		t.Fatalf("cron %q, time %q, days %v after editing the words", row.CronExpr, saved.Time, saved.Days)
	}
}

func TestChangingTheTriggerIsSaved(t *testing.T) {
	st := newTestStore(t)
	svc := schedules.NewService(st, nil)
	ctx := context.Background()
	made, _ := svc.Save(ctx, protocol.SaveScheduleRequest{Name: "A", Kind: "brief", Trigger: "Cron", When: "Every day at 9:00"})
	saved, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		ID: made.ID, Name: "A", Kind: "brief", Trigger: "Event", When: "When 30 minutes before my first calendar event",
	})
	if err != nil || saved.Trigger != "Event" {
		t.Fatalf("the trigger after an edit is %q (err %v), want Event", saved.Trigger, err)
	}
}

func TestTheTemplateSectionsChannelsAndQuietFlagAreSavedAndKeptThroughAnEdit(t *testing.T) {
	st := newTestStore(t)
	svc := schedules.NewService(st, nil)
	ctx := context.Background()
	made, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		Name: "Morning brief", Kind: "brief", Trigger: "Cron", When: "Every day at 8:00", Template: "morning",
		Sections: []string{"calendar", "needs-you"}, Deliver: []string{"telegram"}, QuietWhenEmpty: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if made.Template != "morning" || !slices.Equal(made.Sections, []string{"calendar", "needs-you"}) ||
		!slices.Equal(made.Deliver, []string{"telegram"}) || !made.QuietWhenEmpty {
		t.Fatalf("the new fields were not saved: %+v", made)
	}
	// An edit that names other parts changes them, and the template stays what the schedule began as.
	edited, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		ID: made.ID, Name: "Mine", Kind: "brief", Trigger: "Cron", When: "Every day at 8:00", Template: "weekly",
		Sections: []string{"finished"}, Deliver: []string{}, QuietWhenEmpty: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Template != "morning" || !slices.Equal(edited.Sections, []string{"finished"}) ||
		len(edited.Deliver) != 0 || edited.QuietWhenEmpty {
		t.Fatalf("the edit did not apply, or changed the template: %+v", edited)
	}
	list, _ := svc.List(ctx, "")
	if list[0].Sections == nil || list[0].Deliver == nil {
		t.Fatal("a list carried a null sections or deliver")
	}
}

func TestARunWithNoHandlerSaysItDidNothing(t *testing.T) {
	st := newTestStore(t)
	clock := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return clock }))
	ctx := context.Background()
	made, _ := svc.Save(ctx, protocol.SaveScheduleRequest{
		Name: "Ask the Orchestrator", Kind: "job", Trigger: "Cron", When: "Every 15 minutes",
		Action: "Send a message to the Orchestrator", Enabled: true, Missed: "Run once on wake",
	})
	clock = clock.Add(time.Hour)
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer svc.Stop()
	runs, _ := svc.Runs(ctx, made.ID)
	if len(runs) != 1 || runs[0].Status != schedules.StatusUnsupported || !strings.Contains(runs[0].Details, "nothing was done") {
		t.Fatalf("a job nothing can run recorded %+v", runs)
	}
}

func TestAHandlerThatFailsKeepsWhatItWroteBesideTheError(t *testing.T) {
	st := newTestStore(t)
	clock := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return clock }))
	svc.RegisterHandler("brief", func(context.Context, protocol.Schedule, time.Time) (string, error) {
		return "the brief text", errors.New("Telegram refused it")
	})
	ctx := context.Background()
	made, _ := svc.Save(ctx, protocol.SaveScheduleRequest{
		Name: "B", Kind: "brief", Trigger: "Cron", When: "Every 15 minutes", Enabled: true, Missed: "Run once on wake",
	})
	clock = clock.Add(time.Hour)
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer svc.Stop()
	runs, _ := svc.Runs(ctx, made.ID)
	if len(runs) != 1 || runs[0].Status != "failed" ||
		!strings.Contains(runs[0].Details, "the brief text") || !strings.Contains(runs[0].Details, "Telegram refused it") {
		t.Fatalf("a failed brief recorded %+v", runs)
	}
}

func TestTheStartersAreMadeOnceSwitchedOffAndAreNotMadeAgainAfterOneIsDeleted(t *testing.T) {
	st := newTestStore(t)
	svc := schedules.NewService(st, nil)
	ctx := context.Background()
	if err := svc.EnsureStarters(ctx); err != nil {
		t.Fatal(err)
	}
	list, _ := svc.List(ctx, "")
	if len(list) != len(briefs.Templates()) {
		t.Fatalf("%d starters made, want %d", len(list), len(briefs.Templates()))
	}
	for _, sched := range list {
		if sched.Enabled {
			t.Errorf("starter %q was made switched on", sched.Name)
		}
		if sched.Template == "" || sched.Kind != "brief" || len(sched.Sections) == 0 || sched.Project != "" {
			t.Errorf("starter %q is incomplete: %+v", sched.Name, sched)
		}
		if !slices.Equal(sched.Deliver, []string{"telegram", "discord", "ntfy"}) {
			t.Errorf("starter %q delivers to %v, want every chat", sched.Name, sched.Deliver)
		}
	}
	if err := svc.Delete(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureStarters(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := svc.List(ctx, "")
	if len(after) != len(list)-1 {
		t.Fatalf("a deleted starter came back: %d schedules, want %d", len(after), len(list)-1)
	}
}

func TestAStarterTheCronCanReadHasItsOwnTimeAndDays(t *testing.T) {
	st := newTestStore(t)
	svc := schedules.NewService(st, nil)
	ctx := context.Background()
	if err := svc.EnsureStarters(ctx); err != nil {
		t.Fatal(err)
	}
	rows, _ := st.Queries().ListSchedules(ctx)
	want := map[string]string{
		briefs.TemplateMorning: "0 8 * * 1,2,3,4,5", briefs.TemplateWindDown: "0 18 * * 1,2,3,4,5",
		briefs.TemplateWeekly: "0 18 * * 0", briefs.TemplateStale: "0 9 * * 1", briefs.TemplateFirstMeet: "",
	}
	for _, row := range rows {
		if expr, ok := want[row.Template]; ok && row.CronExpr != expr {
			t.Errorf("%s has the cron %q, want %q", row.Template, row.CronExpr, expr)
		}
		if row.Enabled != 0 {
			t.Errorf("%s is enabled in the store", row.Template)
		}
	}
}

func TestARunByHandIsAPreviewThatLeavesTheLastRunTimeAlone(t *testing.T) {
	st := newTestStore(t)
	svc := schedules.NewService(st, nil)
	var seen []time.Time
	svc.RegisterHandler("brief", func(_ context.Context, _ protocol.Schedule, since time.Time) (string, error) {
		seen = append(seen, since)
		return "the brief", nil
	})
	ctx := context.Background()
	made, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		Name: "Morning brief", Kind: "brief", Trigger: "Cron", When: "Every day at 8:00", Enabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		run, err := svc.RunNow(ctx, made.ID)
		if err != nil || !strings.Contains(run.Details, "Run by hand.") || run.Status != "success" {
			t.Fatalf("the run was %+v (err %v)", run, err)
		}
	}
	// A schedule that has never run has no since, however many times it was tried by hand.
	if len(seen) != 2 || !seen[0].IsZero() || !seen[1].IsZero() {
		t.Fatalf("the handler was given %v, want the zero time both times", seen)
	}
	row, err := st.Queries().GetSchedule(ctx, made.ID)
	if err != nil || row.LastRunAt != 0 {
		t.Fatalf("a run by hand moved the last run time to %d (err %v)", row.LastRunAt, err)
	}
	runs, _ := svc.Runs(ctx, made.ID)
	if len(runs) != 2 {
		t.Fatalf("the history holds %d runs, want both recorded", len(runs))
	}
}

func TestARunByHandDoesNotSwitchOffAOneTimeSchedule(t *testing.T) {
	st := newTestStore(t)
	svc := schedules.NewService(st, nil)
	ctx := context.Background()
	made, _ := svc.Save(ctx, protocol.SaveScheduleRequest{
		Name: "Once", Kind: "job", Trigger: "One-time", When: "On 1 December at 14:00", Enabled: true,
	})
	if _, err := svc.RunNow(ctx, made.ID); err != nil {
		t.Fatal(err)
	}
	row, _ := st.Queries().GetSchedule(ctx, made.ID)
	if row.Enabled != 1 {
		t.Fatal("trying a one-time schedule by hand switched it off before it ever ran on its own")
	}
}

func TestOnlyABriefCanBePreviewedAndAPreviewRecordsNoRun(t *testing.T) {
	st := newTestStore(t)
	svc := schedules.NewService(st, nil)
	var asked []time.Time
	svc.SetPreviewer(func(_ context.Context, _ protocol.Schedule, since time.Time) (string, error) {
		asked = append(asked, since)
		return "Morning brief\n\nNeeds you (1)", nil
	})
	ctx := context.Background()
	brief, _ := svc.Save(ctx, protocol.SaveScheduleRequest{Name: "Morning brief", Kind: "brief", Trigger: "Cron", When: "Every day at 8:00"})
	job, _ := svc.Save(ctx, protocol.SaveScheduleRequest{Name: "Ask", Kind: "job", Trigger: "Cron", When: "Every day at 9:00"})

	got, err := svc.Preview(ctx, brief.ID)
	if err != nil || got.Text != "Morning brief\n\nNeeds you (1)" {
		t.Fatalf("the preview of a brief is %+v (err %v)", got, err)
	}
	if len(asked) != 1 || !asked[0].IsZero() {
		t.Fatalf("a brief that never ran was previewed from %v, want the zero time", asked)
	}
	if runs, _ := svc.Runs(ctx, brief.ID); len(runs) != 0 {
		t.Fatalf("a preview recorded %d runs", len(runs))
	}
	if _, err := svc.Preview(ctx, job.ID); err == nil {
		t.Fatal("a job was previewed")
	}
	if _, err := svc.Preview(ctx, "01H1234567890ABCDEFGHJKMNP"); !errors.Is(err, schedules.ErrNotFound) {
		t.Fatalf("an unknown schedule answered %v, want ErrNotFound", err)
	}
}
