import type { MergePhase } from "@marshal/protocol";
import type { MergeFlow } from "~/data/mappers/integration";
import type { Card, Chat } from "~/mock/types";

/** One lane of cards that are on their way into the branch, in the order a card passes through. */
interface LaneInfo {
  phase: MergePhase;
  title: string;
  icon: string;
}

/** The lanes of the queue, named for what is happening to the cards in them. */
export const ACTIVE_LANES: readonly LaneInfo[] = [
  { phase: "queued", title: "Waiting to merge", icon: "clock" },
  { phase: "resolving", title: "Resolving conflicts", icon: "git-merge" },
  { phase: "testing", title: "Testing", icon: "flask-conical" },
  { phase: "landing", title: "Landing in your folder", icon: "folder-open" },
];

/** What the view says when no card is waiting, stopped, or being merged. */
export const NOTHING_WAITING = "Nothing is waiting to merge.";

/** The store's card for one the daemon names by its own id, or undefined when the store has none. */
export const storeCardOf = (
  cards: readonly Card[],
  projectId: string,
  daemonId: string,
): Card | undefined =>
  cards.find(
    (card) => card.p === projectId && card.daemonId !== undefined && card.daemonId === daemonId,
  );

/**
 * The cards whose merge stopped and who wait on the owner. A merge conflict is one; so is the card
 * the Integrator says it is waiting on. A CI failure is not: the CI monitor uses that kind for an
 * ordinary branch too, and a retry of the merge would be the wrong answer to it.
 */
export function stoppedCards(cards: readonly Card[], projectId: string, flow: MergeFlow): Card[] {
  return cards.filter((card) => {
    if (card.p !== projectId || card.state !== "needs") return false;
    if (card.mergePhase === "stopped" || card.reasonKind === "conflict") return true;
    return flow.state === "waiting" && !!card.daemonId && card.daemonId === flow.currentCardId;
  });
}

/** True when no card waits, is being merged, or has stopped. */
export const nothingWaiting = (flow: MergeFlow, stopped: readonly Card[]): boolean =>
  flow.queued === 0 && stopped.length === 0;

/** "Up to date", or how many commits the Integrator's branch has that the target does not. */
export const aheadLabel = (aheadBy: number): string =>
  aheadBy <= 0 ? "Up to date" : `Ahead by ${aheadBy}`;

/** A chat the store marks as the project's pinned Integrator chat, or undefined when it does not. */
export const integratorChatOf = (chats: readonly Chat[] | undefined): Chat | undefined =>
  chats?.find((chat) => (chat as Chat & { system?: string }).system === "integrator");
