import { type Card, type CardTab, M } from "~/mock";

export interface CardTabInfo {
  key: CardTab;
  label: string;
  /** The number shown next to the label, or undefined when there is nothing to count. */
  count: string | number | undefined;
  /** Red count: a check failed. */
  failed: boolean;
}

const TAB_LABELS: readonly [CardTab, string][] = [
  ["chat", "Chat"],
  ["comments", "Comments"],
  ["activity", "Activity"],
  ["diff", "Diff"],
  ["checks", "Checklists"],
  ["preview", "Preview"],
  ["notes", "Notes"],
];

export const TAB_KEYS: readonly CardTab[] = TAB_LABELS.map(([key]) => key);

/** Done and total over the card's checklists and its acceptance checks, as `3/7`. */
function checksCount(card: Card): string {
  const checks = M.S.checks[card.id] ?? [];
  const items = card.checklists.flatMap((list) => list.items);
  const total = items.length + checks.length;
  if (!total) return "";
  const done = items.filter((item) => item.done).length;
  return `${done + checks.filter((k) => k.st === "passed").length}/${total}`;
}

function countOf(card: Card, key: CardTab): string | number | undefined {
  const counts: Partial<Record<CardTab, string | number>> = {
    comments: card.comments.length,
    activity: (M.S.act[card.id] ?? []).length,
    diff: M.diffFor(card).length,
    checks: checksCount(card),
  };
  const count = counts[key];
  return count === "" || count === 0 ? undefined : count;
}

/** The seven tabs with their counts. Read it inside a memo. */
export function cardTabs(card: Card): CardTabInfo[] {
  const failed = (M.S.checks[card.id] ?? []).some((k) => k.st === "failed");
  return TAB_LABELS.map(([key, label]) => ({
    key,
    label,
    count: countOf(card, key),
    failed: key === "checks" && failed,
  }));
}

/** The tab an arrow key moves to from `current`, wrapping around. */
export function neighborTab(current: CardTab, key: "ArrowRight" | "ArrowLeft"): CardTab {
  const index = TAB_KEYS.indexOf(current);
  const step = key === "ArrowRight" ? 1 : TAB_KEYS.length - 1;
  return TAB_KEYS[(index + step) % TAB_KEYS.length] ?? current;
}
