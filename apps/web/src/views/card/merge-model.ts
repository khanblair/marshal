import { NeedsReasonKindCIFailed, NeedsReasonKindConflict } from "@marshal/protocol";
import type { Card } from "~/mock";

/** What a card must say for the Retry button: it waits on a person, and why. */
type Waiting = Pick<Card, "state" | "reasonKind" | "reason" | "mergePhase">;

/**
 * True when the card waits on a person because the merge queue stopped it, so Retry can run the
 * merge again. The daemon marks such a card with the stopped phase; a conflict always comes from the queue. A failed CI also comes from the pull
 * request's own checks, so only the sentence that names the merge counts.
 */
export function isMergeStop(card: Waiting): boolean {
  if (card.state !== "needs") return false;
  if (card.mergePhase === "stopped") return true;
  if (card.reasonKind === NeedsReasonKindConflict) return true;
  return card.reasonKind === NeedsReasonKindCIFailed && /\bmerge/i.test(card.reason);
}

/** A branch as `git branch` writes it: without the `refs/heads/` that a full ref name carries. */
export function shortBranch(name: string | null | undefined): string {
  return (name ?? "").replace(/^refs\/heads\//, "");
}
