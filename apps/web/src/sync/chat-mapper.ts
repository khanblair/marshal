import type {
  ActivityItem,
  ChatApproval,
  ChatMessage,
  ChatPlan,
  ChatToolCall,
} from "@marshal/protocol";
import { toMillis } from "~/data/mappers/time";
import { cardKey } from "~/mock/card-key";
import type { Activity, ActivityKind, ActivityState, Msg, ToolState } from "~/mock/types";

/*
 * A card's chat and activity, from the daemon to the screens.
 *
 * Both sides were made to look alike, so most of this is one word for another; what is here is the
 * few places they are not:
 *
 *   - The wire calls a card's chat "messages" and numbers them newest first, because a page is read
 *     backwards from the newest. The store keeps a card's chat oldest first, which is the order the
 *     screen draws it in, so a page is reversed on the way in.
 *   - The wire has a `thought` kind for the agent's reasoning; the screens draw reasoning as the
 *     agent's own words, so it arrives as an agent message.
 *   - A tool call's outcome is not in the list at all: the page carries one line and a flag saying
 *     there is more, and the rest is read on demand (see `sync/card-session.ts`). The activity list
 *     is where an outcome is drawn, and there the wire says `failed` where the store says `fail`.
 */

/** The icon the screens draw for each tool kind. A kind the app does not know gets the generic one. */
const TOOL_ICONS: Record<string, string> = {
  read: "book-open",
  edit: "pencil",
  delete: "trash-2",
  search: "search",
  execute: "square-terminal",
  switch_mode: "terminal",
};

/** The icon a tool call is drawn with when the daemon reports a kind the screens do not know. */
const GENERIC_TOOL_ICON = "terminal";

const toolIcon = (tool: ChatToolCall): string => TOOL_ICONS[tool.toolKind] ?? GENERIC_TOOL_ICON;

/** How a tool call is going, in the words the store uses. */
const toolState = (state: ChatToolCall["state"]): ToolState => {
  if (state === "running") return "running";
  if (state === "failed") return "fail";
  return "ok";
};

/** A plan's state. The wire can also say the plan was edited, which the store draws beside the state. */
const planState = (plan: ChatPlan): "waiting" | "approved" | "rejected" => {
  if (plan.state === "rejected") return "rejected";
  return plan.state === "waiting" ? "waiting" : "approved";
};

const planEdited = (plan: ChatPlan): boolean => plan.state === "edited";

/** One stored plan, in the shape the plan block draws. */
const planMessage = (message: ChatMessage, plan: ChatPlan): Msg => ({
  id: message.id,
  k: "plan",
  st: planState(plan),
  editing: false,
  ...(planEdited(plan) ? { edited: true } : {}),
  steps: [...plan.steps],
  files: [...plan.files],
  risks: [...plan.risks],
  checks: [...plan.checks],
});

/** One stored approval request, in the shape the approval block draws. */
const approvalMessage = (message: ChatMessage, approval: ChatApproval): Msg => ({
  id: message.id,
  k: "approval",
  st: approval.state,
  cmd: approval.command,
  why: approval.reason,
});

/**
 * One message, in the shape the screens draw. A message kind with no block of its own is drawn as
 * a system note: the store would otherwise show nothing at all, and a person would see a card's
 * chat stop in the middle with no reason.
 */
function toStoredMessage(message: ChatMessage): Msg {
  switch (message.kind) {
    case "user":
      return { id: message.id, k: "user", text: message.text };
    case "agent":
      return { id: message.id, k: "agent", text: message.text };
    case "tool":
      return message.tool
        ? {
            id: message.id,
            call: message.tool.id,
            k: "tool",
            icon: toolIcon(message.tool),
            action: message.tool.title,
            // The outcome is read from the message's own detail when the block is opened; the list
            // carries one line and the state only.
            result: "",
            st: toolState(message.tool.state),
            detail: "",
            open: false,
          }
        : { id: message.id, k: "system", text: message.text };
    case "diff":
      return message.diff
        ? {
            id: message.id,
            k: "diff",
            files: message.diff.files,
            add: message.diff.additions,
            del: message.diff.deletions,
          }
        : { id: message.id, k: "system", text: message.text };
    case "plan":
      return message.plan
        ? planMessage(message, message.plan)
        : { id: message.id, k: "system", text: message.text };
    case "approval":
      return message.approval
        ? approvalMessage(message, message.approval)
        : { id: message.id, k: "system", text: message.text };
    case "card": {
      const cards = message.card?.cards ?? [];
      // A card reference names a card the way the screens do: `<projectId>#<number>`.
      const keys = cards.map((card) => cardKey(card.projectId, card.number));
      if (keys.length === 1 && keys[0]) return { id: message.id, k: "card", cardId: keys[0] };
      return { id: message.id, k: "links", text: message.text, cards: keys };
    }
    default:
      return { id: message.id, k: "system", text: message.text };
  }
}

/**
 * A tool call is stored as two rows: the one that starts it, with its title and its kind, and the one
 * that reports how it ended, with the same call id, no title, and the flag that there is more to
 * read. The screens draw one line per call, so the later row is folded into the earlier one: the line
 * keeps the first row's place and words, takes the last state, and takes the id of the row that has
 * the detail, which is what the detail route is asked with. `oldestFirst` is a page in reading order.
 */
function foldToolCalls(oldestFirst: readonly ChatMessage[]): ChatMessage[] {
  const placeOf = new Map<string, number>();
  const folded: ChatMessage[] = [];
  for (const message of oldestFirst) {
    const call = message.kind === "tool" ? message.tool : null;
    const at = call ? placeOf.get(call.id) : undefined;
    const first = at === undefined ? undefined : folded[at];
    if (!call || at === undefined || !first?.tool) {
      if (call) placeOf.set(call.id, folded.length);
      folded.push(message);
      continue;
    }
    folded[at] = {
      ...message,
      seq: first.seq,
      at: first.at,
      text: message.text || first.text,
      tool: {
        ...call,
        title: call.title || first.tool.title,
        toolKind: call.toolKind || first.tool.toolKind,
        state: call.state ?? first.tool.state ?? null,
        hasDetail: call.hasDetail || first.tool.hasDetail,
      },
    };
  }
  return folded;
}

/**
 * One page of a card's or a project chat's messages, in the order the screens draw it: the daemon
 * answers newest first and the store keeps oldest first, so the page is reversed here, once, and
 * everything downstream reads a list that grows at its end. A tool call's two rows are one line.
 */
export const toStoredMessages = (page: readonly ChatMessage[]): Msg[] =>
  foldToolCalls([...page].reverse()).map(toStoredMessage);

/** How an activity entry ended, in the words the store uses. */
const activityState = (state: ActivityItem["state"]): ActivityState => {
  if (state === "failed") return "fail";
  return state;
};

/** One column of a card's activity, in the shape the screen draws. */
export function toStoredActivity(item: ActivityItem, now: number): Activity {
  return {
    id: item.id,
    kind: item.kind as ActivityKind,
    text: item.text,
    result: item.result,
    st: activityState(item.state),
    ts: toMillis(item.at),
    // The daemon's own "just now" is not known here, and the screens only use it to pulse a fresh
    // entry; a page that was just read is fresh, and the pulse is not what anything asserts.
    ...(now - toMillis(item.at) < FRESH_MS ? { fresh: true } : {}),
  };
}

/** How long after an event its row still pulses, which is what the store's `fresh` means. */
const FRESH_MS = 5_000;

/** One page of a card's activity, in the order the screens draw it: newest first, as the daemon sends it. */
export const toStoredActivityList = (page: readonly ActivityItem[], now: number): Activity[] =>
  page.map((item) => toStoredActivity(item, now));
