package integrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"
)

const (
	// DefaultResolveTimeout is how long Resolve waits for the agent's verdict.
	DefaultResolveTimeout = 15 * time.Minute
	// DefaultResetEvery is how many tasks one Integrator session handles before it starts fresh.
	DefaultResetEvery = 8
	// resetGrace bounds the reset made when a task is given up on.
	resetGrace = 10 * time.Second
	// finishedTTL is how long a reported task is remembered, to answer a late second report.
	finishedTTL = time.Hour
)

var (
	// ErrResolveTimeout is wrapped by the error Resolve returns when no verdict arrived in time.
	ErrResolveTimeout = errors.New("the integrator agent gave no verdict in time")
	// ErrUnknownTask is wrapped when a task id is not waiting: never sent, finished, or timed out.
	ErrUnknownTask = errors.New("no merge task with that id is waiting; it was never sent, has finished, or timed out")
	// ErrAlreadyReported is wrapped when a task already has its verdict.
	ErrAlreadyReported = errors.New("this merge task already has a verdict")
	// ErrInvalidVerdict is wrapped when a verdict cannot be accepted.
	ErrInvalidVerdict = errors.New("the verdict cannot be accepted")
)

// Asker puts a blocking question from the Integrator agent in front of the owner: on the card's
// Needs you and as a message in the Integrator chat. It must not Send into the agent's own session.
type Asker interface {
	Ask(ctx context.Context, projectID, cardID, text string) error
}

// AgentDeps are the parts an AgentResolver is built from.
type AgentDeps struct {
	// Chat is the pinned Integrator chat's session. Required.
	Chat ChatSession
	// Asker delivers ask_owner questions. Nil means ask_owner answers with an error.
	Asker Asker
	// Now reads the clock. Nil means time.Now.
	Now func() time.Time
	// Timeout is how long one task may wait for its verdict. Zero means DefaultResolveTimeout.
	Timeout time.Duration
	// ResetEvery is how many tasks a project's session handles before a reset. Zero means DefaultResetEvery.
	ResetEvery int
	// Log is where problems are written. Nil discards.
	Log *slog.Logger
}

// AgentResolver gives conflicts to the project's one Integrator agent session and waits for its
// verdict. It is a ConflictResolver, and its Context, Report and Ask methods are the MergeTools the
// agent's tools call. Tasks of different projects run at once; one project's are serialized by the caller.
type AgentResolver struct {
	chat       ChatSession
	asker      Asker
	now        func() time.Time
	timeout    time.Duration
	resetEvery int
	log        *slog.Logger

	mu    sync.Mutex
	tasks map[string]*waitingTask
	sent  map[string]int
}

// waitingTask is one task the registry knows, from Resolve's register until the verdict is old news.
type waitingTask struct {
	task     MergeTask
	deadline time.Time
	done     chan struct{}
	verdict  Verdict
	reported bool
	over     bool
}

// NewAgentResolver builds an AgentResolver. The Integrator chat is required.
func NewAgentResolver(deps AgentDeps) (*AgentResolver, error) {
	if deps.Chat == nil {
		return nil, errors.New("the integrator agent needs the Integrator chat")
	}
	r := &AgentResolver{
		chat: deps.Chat, asker: deps.Asker, now: deps.Now, timeout: deps.Timeout,
		resetEvery: deps.ResetEvery, log: deps.Log,
		tasks: map[string]*waitingTask{}, sent: map[string]int{},
	}
	if r.now == nil {
		r.now = time.Now
	}
	if r.timeout <= 0 {
		r.timeout = DefaultResolveTimeout
	}
	if r.resetEvery <= 0 {
		r.resetEvery = DefaultResetEvery
	}
	if r.log == nil {
		r.log = slog.New(slog.DiscardHandler)
	}
	return r, nil
}

// ready is false for a nil or zero-value resolver, so its methods answer an error instead of panicking.
func (r *AgentResolver) ready() bool { return r != nil && r.tasks != nil && r.chat != nil }

// errNotReady is what a resolver that was not built by NewAgentResolver answers.
func errNotReady() error { return errors.New("the integrator agent is not set up") }

// Resolve sends the task to the project's Integrator session and waits for its merge_report, for
// the caller's context to end, or for the timeout. A timeout resets the session at once and a cancel
// before the next task, so an agent still on the abandoned task cannot touch the workspace.
func (r *AgentResolver) Resolve(ctx context.Context, task MergeTask) (Verdict, error) {
	if !r.ready() {
		return Verdict{}, errNotReady()
	}
	w, err := r.register(task)
	if err != nil {
		return Verdict{}, err
	}
	wctx, cancel := context.WithTimeoutCause(ctx, r.timeout, ErrResolveTimeout)
	defer cancel()
	sendErr := r.deliver(wctx, task)
	if sendErr == nil {
		select {
		case <-w.done:
		case <-wctx.Done():
		}
	}
	if verdict, ok := r.settle(w); ok {
		r.log.Info("the integrator agent reported", "task_id", task.ID, "project_id", task.ProjectID,
			"resolved", verdict.Resolved, "confident", verdict.Confident)
		return verdict, nil
	}
	return Verdict{}, r.failure(ctx, wctx, task, sendErr)
}

// register puts the task in the registry, so a report that arrives during Send is not lost.
func (r *AgentResolver) register(task MergeTask) (*waitingTask, error) {
	if task.ID == "" || task.ProjectID == "" {
		return nil, errors.New("a merge task needs an id and a project")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	for id, w := range r.tasks {
		if !now.Before(w.deadline) {
			delete(r.tasks, id)
		}
	}
	if old := r.tasks[task.ID]; old != nil && !old.over {
		return nil, fmt.Errorf("merge task %s is already waiting for a verdict", task.ID)
	}
	w := &waitingTask{task: copyTask(task), deadline: now.Add(r.timeout), done: make(chan struct{})}
	r.tasks[task.ID] = w
	return w, nil
}

// deliver resets the project's session when it is due, then sends the task's message.
func (r *AgentResolver) deliver(ctx context.Context, task MergeTask) error {
	r.resetIfDue(ctx, task.ProjectID)
	if err := r.chat.Send(ctx, task.ProjectID, buildPrompt(task)); err != nil {
		return fmt.Errorf("send task %s to the Integrator chat: %w", task.ID, err)
	}
	r.mu.Lock()
	r.sent[task.ProjectID]++
	r.mu.Unlock()
	return nil
}

// resetIfDue starts the project's session fresh after ResetEvery tasks. A reset that fails is logged
// and left due, so the next task tries again: a session that drifts beats a merge that cannot start.
func (r *AgentResolver) resetIfDue(ctx context.Context, projectID string) {
	r.mu.Lock()
	due := r.sent[projectID] >= r.resetEvery
	r.mu.Unlock()
	if !due {
		return
	}
	if err := r.chat.Reset(ctx, projectID); err != nil {
		r.log.Warn("could not reset the Integrator session", "project_id", projectID, "error", err)
		return
	}
	r.setSent(projectID, 0)
}

// setSent sets how many tasks a project's session has handled since its last reset.
func (r *AgentResolver) setSent(projectID string, n int) {
	r.mu.Lock()
	r.sent[projectID] = n
	r.mu.Unlock()
}

// settle ends the wait on a task. It answers the verdict when one was accepted, even one that came
// in as the wait ended, and keeps that task to answer a late second report.
func (r *AgentResolver) settle(w *waitingTask) (Verdict, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w.over = true
	if w.reported {
		w.deadline = r.now().Add(finishedTTL)
		return w.verdict, true
	}
	if r.tasks[w.task.ID] == w {
		delete(r.tasks, w.task.ID)
	}
	return Verdict{}, false
}

// failure is the error Resolve returns when no verdict came. It resets the session of an abandoned task.
func (r *AgentResolver) failure(ctx, wctx context.Context, task MergeTask, sendErr error) error {
	switch {
	case errors.Is(context.Cause(wctx), ErrResolveTimeout):
		r.abandon(ctx, task.ProjectID)
		return fmt.Errorf("%w: waited %s for task %s", ErrResolveTimeout, r.timeout, task.ID)
	case ctx.Err() != nil:
		r.setSent(task.ProjectID, r.resetEvery)
		return fmt.Errorf("waiting for the integrator agent on task %s: %w", task.ID, ctx.Err())
	case sendErr != nil:
		return sendErr
	}
	return fmt.Errorf("the integrator agent ended task %s without a verdict", task.ID)
}

// abandon resets the project's session now, on a context that outlives the task's. When the reset
// fails the session stays due for one, so the next task starts fresh.
func (r *AgentResolver) abandon(ctx context.Context, projectID string) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resetGrace)
	defer cancel()
	if err := r.chat.Reset(rctx, projectID); err != nil {
		r.log.Warn("could not reset the Integrator session after a timeout", "project_id", projectID, "error", err)
		r.setSent(projectID, r.resetEvery)
		return
	}
	r.setSent(projectID, 0)
}

// copyTask copies a task so the registry and its callers never share a slice.
func copyTask(t MergeTask) MergeTask {
	t.Conflicts = slices.Clone(t.Conflicts)
	t.Cards = slices.Clone(t.Cards)
	for i := range t.Cards {
		t.Cards[i].Changed = slices.Clone(t.Cards[i].Changed)
	}
	return t
}
