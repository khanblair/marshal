import { type Card, M } from "~/mock";

/*
 * What a card that needs you is waiting for, read from its reason text. The
 * icon and the button label test the reasons in different orders, as the
 * design does, so they are two tables.
 */

type Rule = readonly [test: RegExp, result: string];

const REASON_ICONS: readonly Rule[] = [
  [/^Plan/, "list-checks"],
  [/^Approval/, "terminal"],
  [/^CI/, "circle-x"],
  [/conflict/i, "git-merge"],
  [/^Stuck/, "repeat"],
];
const DEFAULT_REASON_ICON = "st-needs";

const REASON_BUTTONS: readonly Rule[] = [
  [/^Plan/, "Review plan"],
  [/^Approval/, "Review command"],
  [/conflict/i, "Resolve conflict"],
  [/^CI/, "See failure"],
];
const DEFAULT_REASON_BUTTON = "Reply";

const firstMatch = (rules: readonly Rule[], reason: string, fallback: string): string =>
  rules.find(([test]) => test.test(reason))?.[1] ?? fallback;

export const reasonIcon = (reason: string): string =>
  firstMatch(REASON_ICONS, reason, DEFAULT_REASON_ICON);

/** The label of the button that opens the card, named after what it asks of you. */
export const reasonButton = (reason: string): string =>
  firstMatch(REASON_BUTTONS, reason, DEFAULT_REASON_BUTTON);

/**
 * Whether the card waits on a command approval, which can be given right from Home.
 *
 * A mock card's waiting approval lives in its open chat (`M.pendingApproval`), which a card nobody
 * has opened has none of - the gap section 3 of the phase report records for S8b. A daemon card
 * carries the answer on the card itself instead (`Card.approvalId`, set from its own NeedsReason),
 * which is what lets Home answer a card it has never opened.
 */
export const canApproveHere = (card: Card): boolean =>
  Boolean(card.approvalId) || M.pendingApproval(card.id)?.k === "approval";

/** `1 card` or `3 cards`. */
export const countLabel = (count: number): string => `${count} ${count === 1 ? "card" : "cards"}`;

/** The line under the heading when nothing waits for you. */
export function nothingNeedsYouText(working: number): string {
  return `Nothing needs you right now. ${working} ${working === 1 ? "agent is" : "agents are"} working.`;
}
