// Package localci runs a project's own workflow steps on this machine
// (docs/marshal-product-scope.md 15.3, docs/backend-checklist.md B6.5, build-plan 6.5): "run the
// same checks as GitHub Actions on your machine, before pushing", which catches a failure sooner
// and saves Actions minutes.
//
// It reads the workflow files of a card's worktree, keeps what Marshal can really run here, and
// reports the rest as `unsupported` with the sentence a person reads. A step that deploys, that
// needs a repository secret, that interpolates a GitHub value, or that is an action rather than a
// command is never run: a local run of it would not be the run GitHub does.
//
// Two rules keep it honest. The reader (`workflow.go`) understands the part of a workflow file
// Marshal needs - the name, the environment, the jobs, and each job's steps with their name,
// command, action, condition, environment, and shell - and records the shapes it does not
// understand (a matrix, a service container, a Windows runner) as a reason a step cannot run,
// never as a guess. The classifier (`classify.go`) decides what a step is, and Marshal runs only
// the test, lint, and build steps, which is what the product scope promises.
//
// Nothing here runs during daemon idle: a run happens only when a route asks for one, every step
// has a time limit, and a step that prints without end has the rest of its output dropped.
package localci

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// WorkflowsDir is where GitHub Actions workflow files live in a repository.
const WorkflowsDir = ".github/workflows"

// Workflow is one workflow file, read as far as Marshal needs it.
type Workflow struct {
	// File is the file's path, relative to the worktree, with forward slashes.
	File string
	// Name is the workflow's own name from its top-level `name:`, or its file name without the
	// extension when it has none.
	Name string
	// Env are the workflow's own environment entries, as KEY=value. An entry whose value Marshal
	// cannot know is left out, and EnvReason says why.
	Env []string
	// EnvReason is why the workflow's own environment is not usable locally. Empty when it is.
	EnvReason string
	// Jobs are the workflow's jobs, in the order the file declares them.
	Jobs []Job
}

// Job is one job of a workflow.
type Job struct {
	// Name is the job's key in the workflow file.
	Name string
	// Env are the job's own environment entries, as KEY=value.
	Env []string
	// EnvReason is why the job's own environment is not usable locally. Empty when it is.
	EnvReason string
	// Skip is why no step of this job can run locally: the job asks for a service container, runs
	// on a matrix or on Windows, or runs only for a GitHub event. Empty when its steps can run.
	Skip string
	// Steps are the job's steps, in the order the file declares them. A job that is not `steps:` at
	// all - one that calls another workflow with `uses:` - has none.
	Steps []Step
}

// Step is one step of one job.
type Step struct {
	// Job is the name of the job this step belongs to.
	Job string
	// Name is the step's own name: its `name:`, or its command when it has none.
	Name string
	// Run is the step's `run:` text, as the file has it. Empty for a step that is an action.
	Run string
	// Uses is the step's `uses:` action, such as "actions/checkout@v4". Empty for a command.
	Uses string
	// If is the step's `if:` condition, as the file has it. Empty when it has none.
	If string
	// Shell is the step's `shell:`, such as "bash". Empty when the file names none, which is how
	// GitHub's own `bash -e` default is chosen.
	Shell string
	// Env are the step's own environment entries, as KEY=value.
	Env []string
	// EnvReason is why the step's own environment is not usable locally. Empty when it is.
	EnvReason string
}

// ReadWorkflows reads every workflow file of a worktree, in file order. A worktree with no
// workflow folder has none, which is not an error.
func ReadWorkflows(dir string) ([]Workflow, error) {
	folder := filepath.Join(dir, WorkflowsDir)
	entries, err := os.ReadDir(folder)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read the workflow folder: %w", err)
	}
	var out []Workflow
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isWorkflowFile(name) {
			continue
		}
		text, err := os.ReadFile(filepath.Join(folder, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		out = append(out, ParseWorkflow(path.Join(WorkflowsDir, name), string(text)))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out, nil
}

// isWorkflowFile says whether a name is a workflow file: GitHub reads .yml and .yaml.
func isWorkflowFile(name string) bool {
	return strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")
}

// ParseWorkflow reads one workflow file's text. It is exported so a test can read a workflow from a
// string, which is how every case of the reader is covered without a repository.
func ParseWorkflow(file, text string) Workflow {
	w := Workflow{File: file, Name: nameFromFile(file)}
	r := &reader{lines: strings.Split(text, "\n")}
	r.workflow(&w)
	return w
}

// nameFromFile is the name a workflow with no `name:` of its own is called: its file name.
func nameFromFile(file string) string {
	base := path.Base(file)
	return strings.TrimSuffix(strings.TrimSuffix(base, ".yml"), ".yaml")
}

// A reader walks a file's lines once, with indentation telling it where it is. It is a small
// YAML-shaped reader rather than a general one: it reads the keys Marshal needs, and everything it
// does not understand is skipped whole rather than guessed at.
type reader struct {
	lines []string
	at    int
}

// line is one line that carries something.
type line struct {
	// index is where the line is in the file.
	index int
	// indent is how many spaces the line is indented by.
	indent int
	// text is the line without its indentation or a trailing comment.
	text string
}

// peek returns the next line that carries anything, without consuming it. Blank lines and comment
// lines are skipped, so a blank line inside a block scalar is not kept.
func (r *reader) peek() (line, bool) {
	for i := r.at; i < len(r.lines); i++ {
		raw := strings.TrimRight(r.lines[i], " \t\r")
		if text := strings.TrimSpace(stripComment(raw)); text != "" {
			return line{index: i, indent: indentOf(raw), text: text}, true
		}
	}
	return line{}, false
}

// take consumes a line that peek returned. It is safe to call for a line already consumed.
func (r *reader) take(l line) { r.at = l.index + 1 }

// skipChildren reads past every line deeper than the given indentation, which is what a key with a
// block Marshal does not read leaves behind. It stops at the next line at or above that
// indentation, so it never skips a sibling key.
func (r *reader) skipChildren(at int) {
	for {
		l, ok := r.peek()
		if !ok || l.indent <= at {
			return
		}
		r.take(l)
	}
}

// childIndent answers the indentation of the block that follows a key at the given indentation: the
// indentation of the next line when it is deeper than the key. It answers false when it is not,
// which means the key held a plain value rather than a block.
func (r *reader) childIndent(at int) (int, bool) {
	l, ok := r.peek()
	if !ok || l.indent <= at {
		return 0, false
	}
	return l.indent, true
}

// workflow reads the top level: the workflow's name and environment, and its jobs.
func (r *reader) workflow(w *Workflow) {
	for {
		l, ok := r.peek()
		if !ok {
			return
		}
		r.take(l)
		key, value, isKey := splitKey(l.text)
		if !isKey {
			r.skipChildren(l.indent)
			continue
		}
		switch key {
		case "name":
			if v, block := r.scalar(value, l); v != "" && !block {
				w.Name = v
			}
			r.skipChildren(l.indent)
		case "env":
			w.Env, w.EnvReason = r.envBlock(value, l.indent)
		case "jobs":
			if at, ok := r.childIndent(l.indent); ok {
				w.Jobs = r.jobs(at)
			}
		default:
			// `on:`, `permissions:`, `concurrency:` and the like: read past their children, which
			// are indented under them and would otherwise look like jobs.
			r.skipChildren(l.indent)
		}
	}
}

// jobs reads every job of a `jobs:` block whose job keys are at the given indentation.
func (r *reader) jobs(at int) []Job {
	var out []Job
	for {
		l, ok := r.peek()
		if !ok || l.indent < at {
			return out
		}
		if l.indent > at {
			// Deeper than a job's own key: not a job. Skip that whole subtree rather than guess.
			r.skipChildren(at)
			continue
		}
		r.take(l)
		name, _, isKey := splitKey(l.text)
		if !isKey {
			continue
		}
		job := Job{Name: name}
		r.jobBody(&job, l.indent)
		out = append(out, job)
	}
}

// jobBody reads one job's keys: its own environment, its steps, and the shapes that stop it from
// running locally.
func (r *reader) jobBody(job *Job, at int) {
	child, ok := r.childIndent(at)
	if !ok {
		return
	}
	for {
		l, ok := r.peek()
		if !ok || l.indent < child {
			return
		}
		if l.indent > child {
			r.skipChildren(child)
			continue
		}
		r.take(l)
		key, value, isKey := splitKey(l.text)
		if !isKey {
			continue
		}
		switch key {
		case "steps":
			if stepAt, ok := r.childIndent(l.indent); ok {
				job.Steps = r.steps(job.Name, stepAt, l.indent)
			}
		case "env":
			job.Env, job.EnvReason = r.envBlock(value, l.indent)
		case "runs-on":
			job.Skip = orReason(job.Skip, runnerReason(r.runnerValue(value, l)))
		case "if":
			job.Skip = orReason(job.Skip, reasonJobCondition)
			r.skipChildren(l.indent)
		case "strategy", "matrix":
			job.Skip = orReason(job.Skip, reasonMatrix)
			r.skipChildren(l.indent)
		case "services", "container", "volumes":
			job.Skip = orReason(job.Skip, reasonService)
			r.skipChildren(l.indent)
		case "uses":
			// A job that calls another workflow is not a set of steps Marshal can run here.
			job.Skip = orReason(job.Skip, reasonReusable)
			r.skipChildren(l.indent)
		default:
			r.skipChildren(l.indent)
		}
	}
}

// steps reads one job's `steps:` block: its items are at `at`, and each step's own keys are two
// spaces deeper. `keyAt` is where the `steps:` key itself sits, which is where the block ends.
func (r *reader) steps(job string, at, keyAt int) []Step {
	var out []Step
	for {
		l, ok := r.peek()
		if !ok || l.indent <= keyAt {
			return out
		}
		if l.indent > at {
			// A block Marshal did not read under a step: skip it rather than mistake its lines for
			// steps of their own.
			r.skipChildren(at)
			continue
		}
		if l.indent < at || !strings.HasPrefix(l.text, "-") {
			return out
		}
		r.take(l)
		step := Step{Job: job}
		// A step may be written on the "- " line itself ("- uses: actions/checkout@v4") or on the
		// lines under it, or both.
		if rest := strings.TrimSpace(strings.TrimPrefix(l.text, "-")); rest != "" {
			r.stepKey(&step, line{index: l.index, indent: at + 2, text: rest})
		}
		r.stepBody(&step, at)
		if step.Name == "" {
			step.Name = stepName(step)
		}
		out = append(out, step)
	}
}

// stepBody reads the keys of one step, which are indented under its first line.
func (r *reader) stepBody(step *Step, at int) {
	keyAt := at + 2
	for {
		l, ok := r.peek()
		if !ok || l.indent < keyAt {
			return
		}
		if l.indent > keyAt {
			r.skipChildren(keyAt)
			continue
		}
		r.take(l)
		r.stepKey(step, l)
	}
}

// stepKey reads one key of a step.
func (r *reader) stepKey(step *Step, l line) {
	key, value, isKey := splitKey(l.text)
	if !isKey {
		r.skipChildren(l.indent)
		return
	}
	switch key {
	case "name":
		if v, block := r.scalar(value, l); v != "" && !block {
			step.Name = v
		}
		r.skipChildren(l.indent)
	case "run":
		step.Run, _ = r.scalar(value, l)
	case "uses":
		step.Uses = r.scalarText(value, l)
		r.skipChildren(l.indent)
	case "if":
		step.If = r.scalarText(value, l)
		r.skipChildren(l.indent)
	case "shell":
		step.Shell = r.scalarText(value, l)
		r.skipChildren(l.indent)
	case "with":
		r.skipChildren(l.indent)
	case "env":
		step.Env, step.EnvReason = r.envBlock(value, l.indent)
	default:
		r.skipChildren(l.indent)
	}
}

// runnerValue answers what a job's `runs-on:` names. A plain value is the runner's name; a value
// written as a block list is its items joined, which is enough to tell a runner Marshal can use
// from one it cannot:
//
//	runs-on:
//	  - self-hosted
//	  - linux
func (r *reader) runnerValue(value string, l line) string {
	if text := strings.TrimSpace(r.scalarText(value, l)); text != "" {
		r.skipChildren(l.indent)
		return text
	}
	return r.listText(l.indent)
}

// listText reads a block list at the given indentation as one text, so a list of labels reads as
// the labels it holds. It consumes the block.
func (r *reader) listText(at int) string {
	child, ok := r.childIndent(at)
	if !ok {
		return ""
	}
	var parts []string
	for {
		l, ok := r.peek()
		if !ok || l.indent < child {
			return strings.Join(parts, " ")
		}
		if l.indent > child {
			r.skipChildren(child)
			continue
		}
		r.take(l)
		if item := strings.TrimSpace(strings.TrimPrefix(l.text, "-")); item != "" {
			parts = append(parts, item)
		}
	}
}

// envBlock reads a key's environment. A key that held a plain value - an inline map or a list - is
// not a block of entries Marshal can read, so it answers the reason rather than nothing at all.
func (r *reader) envBlock(value string, at int) ([]string, string) {
	if strings.TrimSpace(value) != "" {
		r.skipChildren(at)
		return nil, reasonEnvironment
	}
	return r.envMap(at)
}

// envMap reads a `KEY: value` block under a key at the given indentation and answers the entries
// that are plain text, which are the ones a child process can be given. A value Marshal cannot
// know - one that interpolates a GitHub value, a map, or a list - is left out, and the reason says
// so, so a step that needs it is reported rather than run without it.
func (r *reader) envMap(at int) ([]string, string) {
	child, ok := r.childIndent(at)
	if !ok {
		return nil, ""
	}
	var out []string
	reason := ""
	for {
		l, ok := r.peek()
		if !ok || l.indent < child {
			return out, reason
		}
		if l.indent > child {
			r.skipChildren(child)
			continue
		}
		r.take(l)
		key, value, isKey := splitKey(l.text)
		if !isKey {
			// A list of entries rather than a map, which Marshal does not read.
			reason = orReason(reason, reasonEnvironment)
			r.skipChildren(l.indent)
			continue
		}
		if value == "" || isBlockIndicator(value) {
			reason = orReason(reason, reasonEnvironment)
			r.skipChildren(l.indent)
			continue
		}
		if text := r.scalarText(value, l); strings.Contains(text, interpolation) {
			reason = orReason(reason, reasonEnvironment)
			continue
		}
		out = append(out, key+"="+unquote(value))
	}
}

// scalar answers a key's value, reading a block scalar (`|`, `>`) from the lines under it. The
// second answer says whether the value was a block, which a name may not be: a name written as a
// block is a shape Marshal does not read, and the file's own name stands in for it.
func (r *reader) scalar(value string, l line) (string, bool) {
	if !isBlockIndicator(value) {
		return unquote(value), false
	}
	r.take(l)
	folded := strings.HasPrefix(value, ">")
	var parts []string
	for {
		next, ok := r.peek()
		if !ok || next.indent <= l.indent {
			return joinBlock(parts, folded), true
		}
		// A block scalar's lines are the file's own text: a "#" in one is content, not a comment.
		parts = append(parts, blockText(r.lines[next.index], l.indent+2))
		r.take(next)
	}
}

// blockText is one line of a block scalar, with the block's own indentation taken off.
func blockText(raw string, indent int) string {
	trimmed := strings.TrimRight(raw, " \t\r")
	if len(trimmed) <= indent {
		return ""
	}
	return trimmed[indent:]
}

// joinBlock joins a block scalar's lines: a folded block joins them with spaces, and a literal one
// keeps the lines.
func joinBlock(parts []string, folded bool) string {
	sep := "\n"
	if folded {
		sep = " "
	}
	return strings.TrimSpace(strings.Join(parts, sep))
}

// scalarText answers a key's value, reading a block scalar if that is what it is. It is for the
// values a condition, an action, a shell, or an environment entry may be: the text is joined rather
// than refused, because what follows decides what to do with it.
func (r *reader) scalarText(value string, l line) string {
	text, _ := r.scalar(value, l)
	return text
}

// splitKey splits `key: value` into its key and value. It reports false for a line that is not a
// key at all, such as a list item.
func splitKey(text string) (key, value string, ok bool) {
	if strings.HasPrefix(text, "-") {
		return "", "", false
	}
	key, value, found := strings.Cut(text, ":")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, " \t\"'") {
		return "", "", false
	}
	return key, strings.TrimSpace(value), true
}

// isBlockIndicator says whether a value introduces a block scalar.
func isBlockIndicator(value string) bool {
	switch value {
	case "|", "|-", "|+", ">", ">-", ">+":
		return true
	}
	return false
}

// interpolation is how a workflow file names a value only GitHub has, such as `${{ secrets.TOKEN }}`
// or `${{ matrix.os }}`. A step or an environment entry that uses one is reported rather than run
// with it left in.
const interpolation = "${{"

// indentOf counts a line's leading spaces. A tab counts as one, because a workflow file that uses
// one is already unusual and Marshal would rather read it than refuse the file.
func indentOf(raw string) int {
	n := 0
	for _, r := range raw {
		if r != ' ' && r != '\t' {
			break
		}
		n++
	}
	return n
}

// stripComment removes a trailing comment from a line. A "#" is only a comment when it starts the
// content or follows a space, so a value such as `actions/checkout@#v4` keeps its text.
func stripComment(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return ""
	}
	if i := strings.Index(line, " #"); i >= 0 {
		return strings.TrimRight(line[:i], " \t")
	}
	if i := strings.Index(line, "\t#"); i >= 0 {
		return strings.TrimRight(line[:i], " \t")
	}
	return line
}

// unquote trims spaces and one pair of quotes, so a quoted value reads as its text.
func unquote(text string) string {
	text = strings.TrimSpace(text)
	if len(text) >= 2 {
		for _, pair := range [][2]string{{`"`, `"`}, {`'`, `'`}} {
			if strings.HasPrefix(text, pair[0]) && strings.HasSuffix(text, pair[1]) {
				return text[1 : len(text)-1]
			}
		}
	}
	return text
}
