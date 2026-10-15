import { createEffect, on, onCleanup } from "solid-js";
import { type Card, M } from "~/mock";
import type { DiffList } from "./diff-list";

/** How long the card's changes must be quiet before the diff is read again. */
const QUIET_MS = 1500;

/**
 * Keeps the open card's diff current while its agent works. Every tool call and message the agent
 * produces lands in the card's chat, so the chat growing is the sign that files may have changed; the
 * diff is read again once it has been quiet for a moment, so a burst of edits is one read. The first
 * read is the card panel's own and is not repeated here.
 */
export function createLiveDiff(card: () => Card, diff: DiffList): void {
  createEffect(
    on(
      () => [card().id, M.S.chat[card().id]?.length ?? 0, card().upd] as const,
      (_now, before) => {
        if (before === undefined) return;
        const timer = setTimeout(() => diff.refetch(), QUIET_MS);
        onCleanup(() => clearTimeout(timer));
      },
    ),
  );
}
