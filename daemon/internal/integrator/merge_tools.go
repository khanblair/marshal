package integrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// The Integrator agent's three internal tools. The chats workstream registers them on the MCP server
// of the pinned Integrator chat, under exactly these names, argument names and descriptions;
// AgentResolver's Context, Report and Ask methods answer them. Argument names are snake_case, as the
// prompt (prompt.go) and the Integrator role's instructions say them; keep all three in step.
//
// merge_context
//
//	description: "The full task you were sent: the conflicted files and, for every card involved, its
//	  title, task, plan, handoff note, branch, and changed files. Call it before you resolve anything."
//	arguments:   MergeContextArgs {"task_id": string}, required
//	answer:      TaskView(task), a MergeTaskView
//
// merge_report
//
//	description: "Report the verdict on a merge task. Call it once, when every conflicted file is
//	  resolved and staged with git add, or when you cannot. If you are not sure, set confident to
//	  false and put your questions in questions."
//	arguments:   MergeReportArgs {"task_id": string, "resolved": bool, "confident": bool,
//	  "summary": string, "files": [string], "questions": [string]}; task_id, resolved and confident required.
//	  A verdict with resolved false must carry a summary or questions.
//	answer:      {"recorded": true}
//
// ask_owner
//
//	description: "Ask the owner a question that blocks you on a merge task. It is posted in the
//	  Integrator chat and on the card's Needs you. You must still finish with merge_report."
//	arguments:   AskOwnerArgs {"task_id": string, "question": string}, both required
//	answer:      {"asked": true}
//
// A tool that fails answers the error text; ErrUnknownTask, ErrAlreadyReported and ErrInvalidVerdict
// are written for the agent to read.

// MergeContextArgs is merge_context's argument.
type MergeContextArgs struct {
	TaskID string `json:"task_id"`
}

// MergeReportArgs is merge_report's arguments.
type MergeReportArgs struct {
	TaskID    string   `json:"task_id"`
	Resolved  bool     `json:"resolved"`
	Confident bool     `json:"confident"`
	Summary   string   `json:"summary,omitempty"`
	Files     []string `json:"files,omitempty"`
	Questions []string `json:"questions,omitempty"`
}

// Verdict is the arguments as the Verdict that MergeTools.Report takes.
func (a MergeReportArgs) Verdict() Verdict {
	return Verdict{
		Resolved: a.Resolved, Confident: a.Confident, Summary: a.Summary, Files: a.Files, Questions: a.Questions,
	}
}

// AskOwnerArgs is ask_owner's arguments.
type AskOwnerArgs struct {
	TaskID   string `json:"task_id"`
	Question string `json:"question"`
}

// MergeTaskView is merge_context's answer: a MergeTask with the JSON names the agent reads.
type MergeTaskView struct {
	TaskID      string          `json:"task_id"`
	ProjectID   string          `json:"project_id"`
	Kind        string          `json:"kind"`
	Worktree    string          `json:"worktree"`
	Target      string          `json:"target"`
	Conflicts   []string        `json:"conflicts"`
	Cards       []MergeCardView `json:"cards"`
	Instruction string          `json:"instruction,omitempty"`
}

// MergeCardView is one card in a MergeTaskView.
type MergeCardView struct {
	CardID  string   `json:"card_id"`
	Key     string   `json:"key"`
	Title   string   `json:"title"`
	Body    string   `json:"body"`
	Plan    string   `json:"plan"`
	Handoff string   `json:"handoff"`
	Branch  string   `json:"branch"`
	Changed []string `json:"changed"`
}

// TaskView turns a task into merge_context's answer. Its lists are never null.
func TaskView(task MergeTask) MergeTaskView {
	view := MergeTaskView{
		TaskID: task.ID, ProjectID: task.ProjectID, Kind: string(task.Kind), Worktree: task.Worktree,
		Target: task.Target, Conflicts: append([]string{}, task.Conflicts...),
		Cards: make([]MergeCardView, 0, len(task.Cards)), Instruction: task.Instruction,
	}
	for _, card := range task.Cards {
		view.Cards = append(view.Cards, MergeCardView{
			CardID: card.CardID, Key: card.Key, Title: card.Title, Body: card.Body, Plan: card.Plan,
			Handoff: card.Handoff, Branch: card.Branch, Changed: append([]string{}, card.Changed...),
		})
	}
	return view
}

// live finds a task that is still waiting for its verdict. The caller holds r.mu.
func (r *AgentResolver) live(taskID string) (*waitingTask, error) {
	w := r.tasks[taskID]
	if w == nil || w.over || !r.now().Before(w.deadline) {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTask, taskID)
	}
	return w, nil
}

// Context answers the task the agent was sent, whole.
func (r *AgentResolver) Context(_ context.Context, taskID string) (MergeTask, error) {
	if !r.ready() {
		return MergeTask{}, errNotReady()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	w, err := r.live(taskID)
	if err != nil {
		return MergeTask{}, err
	}
	return copyTask(w.task), nil
}

// Report records the agent's verdict and releases the Resolve that waits on the task. A task that is
// not waiting, one that already has a verdict, or a verdict that says nothing is an error, and the
// task keeps waiting so the agent can report again.
func (r *AgentResolver) Report(_ context.Context, taskID string, verdict Verdict) error {
	if !r.ready() {
		return errNotReady()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if w := r.tasks[taskID]; w != nil && w.reported {
		return fmt.Errorf("%w: %s", ErrAlreadyReported, taskID)
	}
	w, err := r.live(taskID)
	if err != nil {
		return err
	}
	clean, err := cleanVerdict(verdict)
	if err != nil {
		return err
	}
	w.verdict, w.reported = clean, true
	close(w.done)
	return nil
}

// Ask posts the agent's question to the owner, for the card being merged. The wait for a verdict
// goes on, and is not extended: an answer that takes longer than the timeout is a timed-out task.
func (r *AgentResolver) Ask(ctx context.Context, taskID, question string) error {
	if !r.ready() {
		return errNotReady()
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return errors.New("ask_owner needs a question")
	}
	r.mu.Lock()
	w, err := r.live(taskID)
	var projectID, cardID string
	if err == nil {
		projectID = w.task.ProjectID
		if len(w.task.Cards) > 0 {
			cardID = w.task.Cards[0].CardID
		}
	}
	r.mu.Unlock()
	if err != nil {
		return err
	}
	if r.asker == nil {
		return errors.New("the owner cannot be asked from here: nothing delivers the question")
	}
	if err := r.asker.Ask(ctx, projectID, cardID, question); err != nil {
		return fmt.Errorf("ask the owner about task %s: %w", taskID, err)
	}
	return nil
}

// cleanVerdict checks a verdict and answers it with its text trimmed and its lists copied.
func cleanVerdict(v Verdict) (Verdict, error) {
	v.Summary = strings.TrimSpace(v.Summary)
	v.Files = nonBlank(v.Files)
	v.Questions = nonBlank(v.Questions)
	if !v.Resolved && v.Summary == "" && len(v.Questions) == 0 {
		return Verdict{}, fmt.Errorf("%w: a verdict that did not resolve the conflicts needs a summary or questions",
			ErrInvalidVerdict)
	}
	return v, nil
}

// nonBlank trims each line and drops the empty ones.
func nonBlank(in []string) []string {
	var out []string
	for _, line := range in {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
