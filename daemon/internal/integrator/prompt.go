package integrator

import (
	"fmt"
	"strings"
)

// buildPrompt is the message the Integrator agent is sent for one task. It stands on its own, so a
// session that was just reset has every rule, and one that was not does not carry an old task over.
func buildPrompt(task MergeTask) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are the Integrator for project %s. This is merge task %s.\n\n", task.ProjectID, task.ID)
	writeSituation(&b, task)
	writeInvolved(&b, task)
	writeSteps(&b, task)
	writeRules(&b)
	writeReportAsk(&b, task)
	return b.String()
}

// writeSituation says what stopped and where the agent works.
func writeSituation(b *strings.Builder, task MergeTask) {
	target := strings.TrimSpace(task.Target)
	if target == "" {
		target = "the integration branch"
	}
	if task.Kind == MergeTaskWIP {
		fmt.Fprintf(b, "%s just moved forward. The owner has uncommitted work that now conflicts with what was "+
			"merged, and the conflict is waiting in your workspace. The conflicting side is THE OWNER'S "+
			"UNCOMMITTED WORK, not a card.\n", target)
	} else {
		fmt.Fprintf(b, "Merging %s into %s stopped on conflicts. The merge is waiting, conflicted, in your workspace.\n",
			firstCardName(task), target)
	}
	fmt.Fprintf(b, "You work only in the integrator workspace, %s. Do not read or change the owner's project "+
		"folder or any other path.\n\n", task.Worktree)
}

// firstCardName names the card being merged, or says it is a card when the task carries none.
func firstCardName(task MergeTask) string {
	if len(task.Cards) == 0 {
		return "a card's branch"
	}
	card := task.Cards[0]
	return fmt.Sprintf("card %s %q (branch %s)", card.Key, card.Title, card.Branch)
}

// writeInvolved lists the conflicted files, the cards, and anything the owner said.
func writeInvolved(b *strings.Builder, task MergeTask) {
	b.WriteString("Conflicted files:\n")
	if len(task.Conflicts) == 0 {
		b.WriteString("- none are listed; run git status in the workspace to find them\n")
	}
	for _, file := range task.Conflicts {
		fmt.Fprintf(b, "- %s\n", file)
	}
	if len(task.Cards) > 0 {
		b.WriteString("\nCards involved, the one being merged first:\n")
		for _, card := range task.Cards {
			fmt.Fprintf(b, "- %s %q, branch %s, %d changed files\n", card.Key, card.Title, card.Branch, len(card.Changed))
		}
	}
	if text := strings.TrimSpace(task.Instruction); text != "" {
		fmt.Fprintf(b, "\nThe owner told the Integrator chat this about the merge:\n%s\n", text)
	}
	b.WriteString("\n")
}

// writeSteps is how to resolve, with the extra step a WIP task has.
func writeSteps(b *strings.Builder, task MergeTask) {
	b.WriteString("How to resolve it:\n")
	fmt.Fprintf(b, "1. Call merge_context with {\"task_id\": %q} for the full task: each card's task, plan, "+
		"handoff note, and changed files.\n", task.ID)
	b.WriteString("2. Read BOTH sides before you touch a file: each card's task and plan, and its diff. " +
		"git diff, git log and git show only read, so they are fine.\n")
	b.WriteString("3. Resolve each conflicted file by intent, not by picking one side's text: keep what each side " +
		"set out to do. Edit the file to its final content with no conflict markers left, then run git add on it.\n")
	if task.Kind == MergeTaskWIP {
		b.WriteString("4. The conflicting side is the owner's uncommitted work, so preserve both sides. Where they " +
			"truly cannot both stand, prefer the owner's version and say so in your summary. Never stash, reset, " +
			"restore, clean or check out anything: that can destroy the owner's work.\n")
	}
	b.WriteString("\n")
}

// writeRules is what the agent must never do, whatever the task.
func writeRules(b *strings.Builder) {
	b.WriteString("Rules:\n")
	b.WriteString("- Do NOT commit, switch branches, push, rebase, tag, or run any command that moves a branch.\n")
	b.WriteString("- Do NOT run git merge --abort or --continue, stash, reset, restore, checkout or clean. " +
		"Marshal finishes the merge after your report.\n")
	b.WriteString("- Change only what resolving the conflicts needs.\n\n")
}

// writeReportAsk is how to report, and what to do when unsure.
func writeReportAsk(b *strings.Builder, task MergeTask) {
	b.WriteString("Finish by calling merge_report with task_id, resolved, confident, summary, files, and questions:\n")
	b.WriteString("- resolved: true only when every conflicted file is resolved and staged.\n")
	b.WriteString("- confident: false when you want the owner to look, even though you resolved it.\n")
	b.WriteString("- summary: what clashed and how you resolved it, in plain words the owner can read.\n")
	b.WriteString("- files: the files you changed.\n")
	b.WriteString("- questions: what you need the owner to answer, when you cannot decide.\n\n")
	b.WriteString("If you are not sure, set confident to false and put your questions in questions. " +
		"If you cannot go on without an answer, call ask_owner with task_id and question first, " +
		"and still finish with merge_report.\n")
	fmt.Fprintf(b, "Report only for task %s. Ignore any earlier task.\n", task.ID)
}
