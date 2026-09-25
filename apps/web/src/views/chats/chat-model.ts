/**
 * Pure helpers for the Chats view: the labels, filters, and option lists the design's
 * `renderVals` computes. They read the store through `M` and hold no state of their own.
 */

import type { Chat, Msg } from "~/mock";
import { M } from "~/mock";
import { cardLabel, cardNumber, parseCardKey } from "~/mock/card-key";

export const ORCHESTRATOR = "Orchestrator";

/** Quick sends above the composer. */
export const SUGGESTIONS = [
  "What is blocked?",
  "Make cards for the export work",
  "Merge the next ready card",
] as const;

/** Where the message list counts as scrolled away from the newest message, in px. */
export const AWAY_THRESHOLD_PX = 80;

/** A chat that talks to a card has the card key as its target, such as `web#118`. */
const isCardTarget = (target: string): boolean => parseCardKey(target) !== null;

/** What a target is called in text: a card key reads `#118`, a role reads as it is. */
const targetName = (target: string): string =>
  isCardTarget(target) ? cardLabel({ n: cardNumber(target) }) : target;

/** The card a key target points at, if it still exists. */
const targetCard = (target: string) => (isCardTarget(target) ? M.card(target) : undefined);

/** The row label of a chat's target: `#118 Agent name`, `Orchestrator`, or `Tester role`. */
export function targetLabel(target: string): string {
  if (isCardTarget(target)) {
    const card = M.card(target);
    return card ? `${cardLabel(card)} ${card.agent}` : targetName(target);
  }
  return target === ORCHESTRATOR ? ORCHESTRATOR : `${target} role`;
}

export function targetIcon(target: string): string {
  if (isCardTarget(target)) return "bot";
  return target === ORCHESTRATOR ? "route" : "user-cog";
}

/** The small facts under the chat title. */
export function targetBits(chat: Chat): string[] {
  const card = targetCard(chat.target);
  if (card) return [card.agent, card.model, card.asleep ? "Asleep" : "Awake"];
  if (chat.target === ORCHESTRATOR) return [ORCHESTRATOR, "claude-opus-4-1", "High thinking"];
  return [`${targetName(chat.target)} role`, "Uses the role template"];
}

export function composerPlaceholder(chat: Chat | undefined): string {
  if (!chat) return "Message";
  const card = targetCard(chat.target);
  if (card) return `Message ${cardLabel(card)}`;
  return `Message ${chat.target === ORCHESTRATOR ? "the Orchestrator" : `the ${chat.target}`}`;
}

const msgText = (msg: Msg): string => ("text" in msg ? msg.text : "");

/** A chat matches a search when its title or any message contains it (case-insensitive). */
export function matchesQuery(chat: Chat, query: string): boolean {
  const q = query.toLowerCase();
  if (!q) return true;
  return (
    chat.title.toLowerCase().includes(q) ||
    chat.msgs.some((msg) => msgText(msg).toLowerCase().includes(q))
  );
}

/** The text of the empty list. The message quotes the query as typed. */
export function noChatsText(query: string): string {
  return query
    ? `No chats match "${query}".`
    : "No chats yet. Start one to plan work with the Orchestrator.";
}

interface TargetOption {
  value: string;
  label: string;
}

/** Who a new chat can talk to: the Orchestrator, each other role, and each awake card's agent. */
export function newChatTargets(pid: string): TargetOption[] {
  const roles = M.ROLE_NAMES.filter((role) => role !== ORCHESTRATOR).map((role) => ({
    value: role,
    label: `${role} role`,
  }));
  const cards = M.cardsOf(pid)
    .filter(M.isAwake)
    .map((card) => ({ value: card.id, label: `${cardLabel(card)} ${card.title}` }));
  return [{ value: ORCHESTRATOR, label: ORCHESTRATOR }, ...roles, ...cards];
}

/**
 * Changes whenever the open chat, its message count, or its last message's length changes.
 * The view scrolls or shows Jump to latest when it does.
 */
export function threadSignature(chat: Chat | undefined): string {
  if (!chat) return ":0:0";
  const last = chat.msgs.at(-1);
  return `${chat.id}:${chat.msgs.length}:${last ? msgText(last).length : 0}`;
}

/** The chat id at the front of a `threadSignature`. */
export const signatureChatId = (signature: string): string => signature.split(":")[0] ?? "";
