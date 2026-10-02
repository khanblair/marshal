package integrator_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrator"
)

// The agent resolver is checked with a fake Integrator chat, a fake Asker, and a fake clock. Nothing
// here starts an agent, runs Git, or waits longer than a few milliseconds on a real timer.

const agentWait = 5 * time.Second

// agentFakeChat records what the resolver does to the Integrator chat, in order.
type agentFakeChat struct {
	mu       sync.Mutex
	events   []string
	texts    []string
	sendFn   func(ctx context.Context, projectID, text string) error
	resetErr error
}

func (c *agentFakeChat) Send(ctx context.Context, projectID, text string) error {
	c.mu.Lock()
	c.events = append(c.events, "send:"+projectID)
	c.texts = append(c.texts, text)
	fn := c.sendFn
	c.mu.Unlock()
	if fn != nil {
		return fn(ctx, projectID, text)
	}
	return nil
}

func (c *agentFakeChat) Reset(_ context.Context, projectID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, "reset:"+projectID)
	return c.resetErr
}

func (c *agentFakeChat) setSend(fn func(ctx context.Context, projectID, text string) error) {
	c.mu.Lock()
	c.sendFn = fn
	c.mu.Unlock()
}

func (c *agentFakeChat) log() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.events)
}

func (c *agentFakeChat) lastText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.texts) == 0 {
		return ""
	}
	return c.texts[len(c.texts)-1]
}

// agentAsk is one question the fake Asker was given.
type agentAsk struct{ project, card, text string }

type agentFakeAsker struct {
	mu   sync.Mutex
	asks []agentAsk
	err  error
}

func (a *agentFakeAsker) Ask(_ context.Context, projectID, cardID, text string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.asks = append(a.asks, agentAsk{projectID, cardID, text})
	return a.err
}

func (a *agentFakeAsker) got() []agentAsk {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.asks)
}

// agentFakeClock is a clock a test moves by hand.
type agentFakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *agentFakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *agentFakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// agentHarness is a resolver over the fakes.
type agentHarness struct {
	chat  *agentFakeChat
	asker *agentFakeAsker
	clock *agentFakeClock
	r     *integrator.AgentResolver
}

// newAgentHarness builds a resolver; tweak, when set, changes the deps before it is built.
func newAgentHarness(t *testing.T, tweak func(*integrator.AgentDeps)) *agentHarness {
	t.Helper()
	h := &agentHarness{
		chat: &agentFakeChat{}, asker: &agentFakeAsker{},
		clock: &agentFakeClock{t: time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC)},
	}
	deps := integrator.AgentDeps{Chat: h.chat, Asker: h.asker, Now: h.clock.Now}
	if tweak != nil {
		tweak(&deps)
	}
	r, err := integrator.NewAgentResolver(deps)
	if err != nil {
		t.Fatalf("build the resolver: %v", err)
	}
	h.r = r
	return h
}

// reportOnSend makes the next Sends of the chat report the verdict for a task, as a fast agent would.
func (h *agentHarness) reportOnSend(taskID string, v integrator.Verdict) {
	h.chat.setSend(func(ctx context.Context, _, _ string) error { return h.r.Report(ctx, taskID, v) })
}

// signalOnSend makes Sends tell the test the task is out, then return, as an agent that is working.
func (h *agentHarness) signalOnSend() <-chan struct{} {
	sent := make(chan struct{}, 16)
	h.chat.setSend(func(context.Context, string, string) error { sent <- struct{}{}; return nil })
	return sent
}

// agentResult is what Resolve answered.
type agentResult struct {
	verdict integrator.Verdict
	err     error
}

// resolveAsync runs Resolve on its own goroutine.
func (h *agentHarness) resolveAsync(ctx context.Context, task integrator.MergeTask) <-chan agentResult {
	out := make(chan agentResult, 1)
	go func() {
		v, err := h.r.Resolve(ctx, task)
		out <- agentResult{v, err}
	}()
	return out
}

func agentTask(id, project string) integrator.MergeTask {
	return integrator.MergeTask{
		ID: id, ProjectID: project, Kind: integrator.MergeTaskCard,
		Worktree: "/data/integrator/" + project, Target: "development",
		Conflicts: []string{"src/a.go", "src/b.go"},
		Cards: []integrator.CardContext{
			{CardID: "card-1", Key: "PROJ#12", Title: "Add login", Plan: "plan one", Branch: "marshal/proj-12", Changed: []string{"src/a.go"}},
			{CardID: "card-2", Key: "PROJ#13", Title: "Add logout", Plan: "plan two", Branch: "marshal/proj-13", Changed: []string{"src/a.go", "src/b.go"}},
		},
	}
}

func agentResolved() integrator.Verdict {
	return integrator.Verdict{Resolved: true, Confident: true, Summary: "kept both", Files: []string{"src/a.go"}}
}

// agentRecv takes one value from a channel or fails the test after agentWait.
func agentRecv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(agentWait):
		t.Fatal("timed out waiting")
		panic("unreachable")
	}
}

// agentNotYet fails the test when the channel already holds a value.
func agentNotYet[T any](t *testing.T, ch <-chan T, what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("%s came too soon", what)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestAgentResolverImplementsTheContracts(t *testing.T) {
	h := newAgentHarness(t, nil)
	var _ integrator.ConflictResolver = h.r
	var _ integrator.MergeTools = h.r
}

func TestNewAgentResolverNeedsTheChat(t *testing.T) {
	if _, err := integrator.NewAgentResolver(integrator.AgentDeps{}); err == nil {
		t.Fatal("a resolver with no chat was built")
	}
}

// A report that comes in while Send is still running must not be lost: the task is registered first.
func TestAgentResolverReportDuringSendReleasesResolve(t *testing.T) {
	h := newAgentHarness(t, nil)
	h.reportOnSend("t1", agentResolved())

	v, err := h.r.Resolve(context.Background(), agentTask("t1", "p1"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !v.Resolved || !v.Confident || v.Summary != "kept both" || !slices.Equal(v.Files, []string{"src/a.go"}) {
		t.Errorf("verdict is %+v", v)
	}
	if got := h.chat.log(); !slices.Equal(got, []string{"send:p1"}) {
		t.Errorf("chat saw %v", got)
	}
}

func TestAgentResolverWaitsForTheAgentsReport(t *testing.T) {
	h := newAgentHarness(t, nil)
	sent := h.signalOnSend()
	res := h.resolveAsync(context.Background(), agentTask("t1", "p1"))
	agentRecv(t, sent)
	agentNotYet(t, res, "a verdict")

	if err := h.r.Report(context.Background(), "t1", agentResolved()); err != nil {
		t.Fatalf("Report: %v", err)
	}
	got := agentRecv(t, res)
	if got.err != nil || !got.verdict.Resolved {
		t.Errorf("Resolve answered %+v", got)
	}
}

func TestAgentResolverNotConfidentVerdictIsReturnedAsIs(t *testing.T) {
	h := newAgentHarness(t, nil)
	h.reportOnSend("t1", integrator.Verdict{Resolved: true, Confident: false, Questions: []string{"  which login wins?  ", " "}})

	v, err := h.r.Resolve(context.Background(), agentTask("t1", "p1"))
	if err != nil {
		t.Fatalf("a not-confident verdict is not an error: %v", err)
	}
	if !v.Resolved || v.Confident || !slices.Equal(v.Questions, []string{"which login wins?"}) {
		t.Errorf("verdict is %+v", v)
	}
}

func TestAgentResolverTimeoutNamesTheTimeoutAndResetsTheSession(t *testing.T) {
	h := newAgentHarness(t, func(d *integrator.AgentDeps) { d.Timeout = 20 * time.Millisecond })

	_, err := h.r.Resolve(context.Background(), agentTask("t1", "p1"))
	if !errors.Is(err, integrator.ErrResolveTimeout) {
		t.Fatalf("error is %v, want a timeout", err)
	}
	if got := err.Error(); !agentContainsAll(got, "t1", "20ms") {
		t.Errorf("the timeout error does not name the task and the wait: %q", got)
	}
	if got := h.chat.log(); !slices.Equal(got, []string{"send:p1", "reset:p1"}) {
		t.Errorf("chat saw %v, want the task and then a reset of the abandoned session", got)
	}
	if err := h.r.Report(context.Background(), "t1", agentResolved()); !errors.Is(err, integrator.ErrUnknownTask) {
		t.Errorf("a late report answered %v, want ErrUnknownTask", err)
	}
}

func TestAgentResolverCancelStopsTheWaitAndResetsBeforeTheNextTask(t *testing.T) {
	h := newAgentHarness(t, nil)
	sent := h.signalOnSend()
	ctx, cancel := context.WithCancel(context.Background())
	res := h.resolveAsync(ctx, agentTask("t1", "p1"))
	agentRecv(t, sent)
	cancel()

	got := agentRecv(t, res)
	if !errors.Is(got.err, context.Canceled) || errors.Is(got.err, integrator.ErrResolveTimeout) {
		t.Fatalf("error is %v, want a cancel and not a timeout", got.err)
	}
	if err := h.r.Report(context.Background(), "t1", agentResolved()); !errors.Is(err, integrator.ErrUnknownTask) {
		t.Errorf("a report after the cancel answered %v, want ErrUnknownTask", err)
	}

	h.reportOnSend("t2", agentResolved())
	if _, err := h.r.Resolve(context.Background(), agentTask("t2", "p1")); err != nil {
		t.Fatalf("the next task: %v", err)
	}
	if got := h.chat.log(); !slices.Equal(got, []string{"send:p1", "reset:p1", "send:p1"}) {
		t.Errorf("chat saw %v, want a reset between the abandoned task and the next", got)
	}
}

func TestAgentResolverResetsAfterEveryNTasksPerProject(t *testing.T) {
	h := newAgentHarness(t, func(d *integrator.AgentDeps) { d.ResetEvery = 2 })
	for _, run := range []struct{ id, project string }{
		{"t1", "p1"}, {"t2", "p1"}, {"t3", "p1"}, {"t4", "p2"},
	} {
		h.reportOnSend(run.id, agentResolved())
		if _, err := h.r.Resolve(context.Background(), agentTask(run.id, run.project)); err != nil {
			t.Fatalf("Resolve %s: %v", run.id, err)
		}
	}
	want := []string{"send:p1", "send:p1", "reset:p1", "send:p1", "send:p2"}
	if got := h.chat.log(); !slices.Equal(got, want) {
		t.Errorf("chat saw %v, want %v", got, want)
	}
}

func TestAgentResolverFailedResetIsLoggedAndRetried(t *testing.T) {
	h := newAgentHarness(t, func(d *integrator.AgentDeps) { d.ResetEvery = 1 })
	h.chat.resetErr = errors.New("boom")
	for _, id := range []string{"t1", "t2", "t3"} {
		h.reportOnSend(id, agentResolved())
		if _, err := h.r.Resolve(context.Background(), agentTask(id, "p1")); err != nil {
			t.Fatalf("Resolve %s: %v", id, err)
		}
	}
	want := []string{"send:p1", "reset:p1", "send:p1", "reset:p1", "send:p1"}
	if got := h.chat.log(); !slices.Equal(got, want) {
		t.Errorf("chat saw %v, want %v", got, want)
	}
}

func TestAgentResolverSendFailureFailsTheTaskAndIsNotCounted(t *testing.T) {
	h := newAgentHarness(t, func(d *integrator.AgentDeps) { d.ResetEvery = 1 })
	h.chat.setSend(func(context.Context, string, string) error { return errors.New("no session") })
	if _, err := h.r.Resolve(context.Background(), agentTask("t1", "p1")); err == nil {
		t.Fatal("Resolve passed although the chat refused the message")
	}
	h.reportOnSend("t2", agentResolved())
	if _, err := h.r.Resolve(context.Background(), agentTask("t2", "p1")); err != nil {
		t.Fatalf("the next task: %v", err)
	}
	if got := h.chat.log(); !slices.Equal(got, []string{"send:p1", "send:p1"}) {
		t.Errorf("chat saw %v, want no reset after a send that failed", got)
	}
}

func TestAgentResolverRunsTwoProjectsAtOnce(t *testing.T) {
	h := newAgentHarness(t, nil)
	var both sync.WaitGroup
	both.Add(2)
	h.chat.setSend(func(ctx context.Context, project, _ string) error {
		both.Done()
		released := make(chan struct{})
		go func() { both.Wait(); close(released) }()
		select {
		case <-released:
		case <-ctx.Done():
			return ctx.Err()
		}
		return h.r.Report(ctx, "task-"+project, integrator.Verdict{Resolved: true, Summary: "for " + project})
	})
	ctx, cancel := context.WithTimeout(context.Background(), agentWait)
	defer cancel()

	one := h.resolveAsync(ctx, agentTask("task-p1", "p1"))
	two := h.resolveAsync(ctx, agentTask("task-p2", "p2"))
	for project, res := range map[string]<-chan agentResult{"p1": one, "p2": two} {
		got := agentRecv(t, res)
		if got.err != nil || got.verdict.Summary != "for "+project {
			t.Errorf("project %s got %+v", project, got)
		}
	}
}

func TestAgentResolverSecondReportIsRefused(t *testing.T) {
	h := newAgentHarness(t, nil)
	sent := h.signalOnSend()
	res := h.resolveAsync(context.Background(), agentTask("t1", "p1"))
	agentRecv(t, sent)

	if err := h.r.Report(context.Background(), "t1", agentResolved()); err != nil {
		t.Fatalf("first Report: %v", err)
	}
	second := integrator.Verdict{Resolved: false, Summary: "changed my mind"}
	if err := h.r.Report(context.Background(), "t1", second); !errors.Is(err, integrator.ErrAlreadyReported) {
		t.Errorf("a second report answered %v, want ErrAlreadyReported", err)
	}
	got := agentRecv(t, res)
	if got.err != nil || !got.verdict.Resolved {
		t.Errorf("Resolve answered %+v, want the first verdict", got)
	}
	if err := h.r.Report(context.Background(), "t1", second); !errors.Is(err, integrator.ErrAlreadyReported) {
		t.Errorf("a report after Resolve returned answered %v, want ErrAlreadyReported", err)
	}
}

func TestAgentResolverRefusesATaskWithoutAnIDOrAProject(t *testing.T) {
	h := newAgentHarness(t, nil)
	for _, task := range []integrator.MergeTask{agentTask("", "p1"), agentTask("t1", "")} {
		if _, err := h.r.Resolve(context.Background(), task); err == nil {
			t.Errorf("Resolve took %+v", task)
		}
	}
	if got := h.chat.log(); len(got) != 0 {
		t.Errorf("chat saw %v for tasks that were refused", got)
	}
}

func TestAgentResolverRefusesAnIDThatIsAlreadyWaiting(t *testing.T) {
	h := newAgentHarness(t, nil)
	sent := h.signalOnSend()
	res := h.resolveAsync(context.Background(), agentTask("t1", "p1"))
	agentRecv(t, sent)

	if _, err := h.r.Resolve(context.Background(), agentTask("t1", "p2")); err == nil {
		t.Error("a second task with the same id was taken")
	}
	if err := h.r.Report(context.Background(), "t1", agentResolved()); err != nil {
		t.Fatalf("Report: %v", err)
	}
	if got := agentRecv(t, res); got.err != nil {
		t.Errorf("Resolve: %v", got.err)
	}
}

func TestAgentResolverNilAndZeroValueAnswerErrors(t *testing.T) {
	var nilR *integrator.AgentResolver
	for name, r := range map[string]*integrator.AgentResolver{"nil": nilR, "zero": {}} {
		ctx := context.Background()
		if _, err := r.Resolve(ctx, agentTask("t1", "p1")); err == nil {
			t.Errorf("%s: Resolve passed", name)
		}
		if _, err := r.Context(ctx, "t1"); err == nil {
			t.Errorf("%s: Context passed", name)
		}
		if err := r.Report(ctx, "t1", agentResolved()); err == nil {
			t.Errorf("%s: Report passed", name)
		}
		if err := r.Ask(ctx, "t1", "why?"); err == nil {
			t.Errorf("%s: Ask passed", name)
		}
	}
}

func agentContainsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
