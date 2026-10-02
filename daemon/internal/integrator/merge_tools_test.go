package integrator_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrator"
)

func TestMergeToolsContextAnswersTheTaskWholeAndByCopy(t *testing.T) {
	h := newAgentHarness(t, nil)
	sent := h.signalOnSend()
	task := agentTask("t1", "p1")
	task.Instruction = "keep the logout flow"
	res := h.resolveAsync(context.Background(), task)
	agentRecv(t, sent)

	got, err := h.r.Context(context.Background(), "t1")
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if got.ID != "t1" || got.Instruction != "keep the logout flow" || len(got.Cards) != 2 ||
		got.Cards[1].Plan != "plan two" || !slices.Equal(got.Conflicts, task.Conflicts) {
		t.Errorf("Context answered %+v", got)
	}
	got.Conflicts[0] = "changed"
	got.Cards[0].Changed[0] = "changed"
	again, _ := h.r.Context(context.Background(), "t1")
	if again.Conflicts[0] != "src/a.go" || again.Cards[0].Changed[0] != "src/a.go" {
		t.Errorf("a caller changed the registered task: %+v", again)
	}

	if err := h.r.Report(context.Background(), "t1", agentResolved()); err != nil {
		t.Fatalf("Report: %v", err)
	}
	agentRecv(t, res)
	if _, err := h.r.Context(context.Background(), "t1"); !errors.Is(err, integrator.ErrUnknownTask) {
		t.Errorf("Context after the task finished answered %v, want ErrUnknownTask", err)
	}
}

func TestMergeToolsUnknownTaskIsRefusedEverywhere(t *testing.T) {
	h := newAgentHarness(t, nil)
	ctx := context.Background()
	if _, err := h.r.Context(ctx, "nope"); !errors.Is(err, integrator.ErrUnknownTask) {
		t.Errorf("Context: %v", err)
	}
	if err := h.r.Report(ctx, "nope", agentResolved()); !errors.Is(err, integrator.ErrUnknownTask) {
		t.Errorf("Report: %v", err)
	}
	if err := h.r.Ask(ctx, "nope", "why?"); !errors.Is(err, integrator.ErrUnknownTask) {
		t.Errorf("Ask: %v", err)
	}
	if got := h.asker.got(); len(got) != 0 {
		t.Errorf("the asker was called for an unknown task: %v", got)
	}
}

func TestMergeToolsExpiredTaskIsRefused(t *testing.T) {
	h := newAgentHarness(t, func(d *integrator.AgentDeps) { d.Timeout = time.Minute })
	sent := h.signalOnSend()
	ctx, cancel := context.WithCancel(context.Background())
	res := h.resolveAsync(ctx, agentTask("t1", "p1"))
	agentRecv(t, sent)

	h.clock.Advance(2 * time.Minute)
	if err := h.r.Report(context.Background(), "t1", agentResolved()); !errors.Is(err, integrator.ErrUnknownTask) {
		t.Errorf("Report after the task expired answered %v, want ErrUnknownTask", err)
	}
	if _, err := h.r.Context(context.Background(), "t1"); !errors.Is(err, integrator.ErrUnknownTask) {
		t.Errorf("Context after the task expired answered %v, want ErrUnknownTask", err)
	}
	cancel()
	agentRecv(t, res)
}

func TestMergeToolsReportWithNothingToSayKeepsTheTaskWaiting(t *testing.T) {
	h := newAgentHarness(t, nil)
	sent := h.signalOnSend()
	res := h.resolveAsync(context.Background(), agentTask("t1", "p1"))
	agentRecv(t, sent)

	for _, bad := range []integrator.Verdict{
		{Resolved: false},
		{Resolved: false, Summary: "  ", Questions: []string{" ", ""}},
		{Resolved: false, Confident: true, Files: []string{"src/a.go"}},
	} {
		if err := h.r.Report(context.Background(), "t1", bad); !errors.Is(err, integrator.ErrInvalidVerdict) {
			t.Errorf("Report(%+v) answered %v, want ErrInvalidVerdict", bad, err)
		}
	}
	agentNotYet(t, res, "a verdict")

	stuck := integrator.Verdict{Resolved: false, Questions: []string{"which login wins?"}}
	if err := h.r.Report(context.Background(), "t1", stuck); err != nil {
		t.Fatalf("Report with questions: %v", err)
	}
	got := agentRecv(t, res)
	if got.err != nil || got.verdict.Resolved || !slices.Equal(got.verdict.Questions, stuck.Questions) {
		t.Errorf("Resolve answered %+v", got)
	}
}

func TestMergeToolsAskPostsToTheFirstCardsNeedsYou(t *testing.T) {
	h := newAgentHarness(t, nil)
	sent := h.signalOnSend()
	res := h.resolveAsync(context.Background(), agentTask("t1", "p1"))
	agentRecv(t, sent)

	if err := h.r.Ask(context.Background(), "t1", "  Keep the old login?  "); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if got, want := h.asker.got(), []agentAsk{{"p1", "card-1", "Keep the old login?"}}; !slices.Equal(got, want) {
		t.Errorf("the asker was given %v, want %v", got, want)
	}
	agentNotYet(t, res, "a verdict")

	if err := h.r.Ask(context.Background(), "t1", "  "); err == nil {
		t.Error("an empty question was asked")
	}
	h.asker.err = errors.New("chat is down")
	if err := h.r.Ask(context.Background(), "t1", "again?"); err == nil || !strings.Contains(err.Error(), "chat is down") {
		t.Errorf("an asker failure answered %v", err)
	}
	if err := h.r.Report(context.Background(), "t1", agentResolved()); err != nil {
		t.Fatalf("Report: %v", err)
	}
	agentRecv(t, res)
}

func TestMergeToolsAskWithoutCardsOrAskerIsHandled(t *testing.T) {
	h := newAgentHarness(t, nil)
	sent := h.signalOnSend()
	task := agentTask("t1", "p1")
	task.Cards = nil
	res := h.resolveAsync(context.Background(), task)
	agentRecv(t, sent)

	if err := h.r.Ask(context.Background(), "t1", "why?"); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if got, want := h.asker.got(), []agentAsk{{"p1", "", "why?"}}; !slices.Equal(got, want) {
		t.Errorf("the asker was given %v, want %v", got, want)
	}

	bare := newAgentHarness(t, func(d *integrator.AgentDeps) { d.Asker = nil })
	bareSent := bare.signalOnSend()
	bareRes := bare.resolveAsync(context.Background(), agentTask("t2", "p1"))
	agentRecv(t, bareSent)
	if err := bare.r.Ask(context.Background(), "t2", "why?"); err == nil {
		t.Error("Ask passed with no asker")
	}
	for _, pair := range []struct {
		h  *agentHarness
		id string
		ch <-chan agentResult
	}{{h, "t1", res}, {bare, "t2", bareRes}} {
		if err := pair.h.r.Report(context.Background(), pair.id, agentResolved()); err != nil {
			t.Fatalf("Report %s: %v", pair.id, err)
		}
		agentRecv(t, pair.ch)
	}
}

func TestTaskViewListsAreNeverNullAndNamesAreSnakeCase(t *testing.T) {
	raw, err := json.Marshal(integrator.TaskView(integrator.MergeTask{ID: "t1", ProjectID: "p1", Kind: integrator.MergeTaskWIP}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"task_id":"t1"`, `"project_id":"p1"`, `"kind":"wip"`, `"conflicts":[]`, `"cards":[]`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the view %s lacks %s", raw, want)
		}
	}
	view := integrator.TaskView(agentTask("t1", "p1"))
	if len(view.Cards) != 2 || view.Cards[1].Key != "PROJ#13" || view.Cards[1].Changed[1] != "src/b.go" {
		t.Errorf("view is %+v", view)
	}
}

func TestMergeReportArgsDecodeAsTheDocumentedShape(t *testing.T) {
	var args integrator.MergeReportArgs
	body := `{"task_id":"t1","resolved":true,"confident":false,"summary":"s","files":["a"],"questions":["q"]}`
	if err := json.Unmarshal([]byte(body), &args); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := integrator.Verdict{Resolved: true, Summary: "s", Files: []string{"a"}, Questions: []string{"q"}}
	got := args.Verdict()
	if args.TaskID != "t1" || got.Resolved != want.Resolved || got.Confident || got.Summary != "s" ||
		!slices.Equal(got.Files, want.Files) || !slices.Equal(got.Questions, want.Questions) {
		t.Errorf("args %+v gave verdict %+v", args, got)
	}
}
