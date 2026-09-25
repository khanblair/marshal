import { batch } from "solid-js";
import { type ApprovalMsg, type Card, type Column, M, type PlanMsg } from "~/mock";

/** One button in the card's action row. */
export interface CardAction {
  label: string;
  icon: string;
  run: () => void;
  /** Ink fill. Only one action leads at a time: the pending approval, else the session control. */
  primary: boolean;
  disabled: boolean;
  /** Keyboard hint drawn inside the button. */
  kbd?: string;
}

export interface MoreItem {
  label: string;
  icon: string;
  danger: boolean;
  run: () => void;
}

type Pending = ApprovalMsg | PlanMsg | undefined;

const action = (
  label: string,
  icon: string,
  run: () => void,
  extra: Partial<CardAction> = {},
): CardAction => ({ label, icon, run, primary: false, disabled: false, ...extra });

function approvalAction(id: number, pending: Pending): CardAction[] {
  if (pending?.k === "approval") {
    return [action("Approve", "check", () => M.approve(id), { primary: true, kbd: "A" })];
  }
  if (pending?.k === "plan") {
    return [action("Approve plan", "check", () => M.approvePlan(id), { primary: true, kbd: "A" })];
  }
  return [];
}

/** Start, resume, wake, or pause: whichever the card's session needs next. */
function sessionAction(card: Card, pending: Pending): CardAction[] {
  const lead = !pending;
  if (card.state === "backlog") {
    return [action("Start card", "play", () => M.start(card.id), { primary: lead })];
  }
  if (card.asleep) {
    return [
      action(card.waking ? "Waking" : "Resume session", "play", () => M.wake(card.id), {
        primary: lead,
        disabled: card.waking,
      }),
    ];
  }
  if (card.paused) {
    return [action("Resume card", "play", () => M.start(card.id), { primary: lead })];
  }
  if (card.state === "working") return [action("Pause", "pause", () => M.pause(card.id))];
  return [];
}

function queueMerge(card: Card): void {
  batch(() => {
    card.state = "merging";
    card.mergePct = 10;
    card.doing = "Dry-run merge with git merge-tree";
    M.toast("Added to merge queue");
  });
}

/** The buttons above the settings, in the design's order. Read it inside a memo. */
export function cardActions(card: Card, mobile: boolean): CardAction[] {
  const id = card.id;
  const pending = M.pendingApproval(id);
  const actions = [...approvalAction(id, pending), ...sessionAction(card, pending)];
  if (card.state !== "backlog" && card.state !== "done" && !card.asleep) {
    actions.push(action("Sleep", "moon", () => M.sleep(id), mobile ? {} : { kbd: "S" }));
  }
  if (card.state !== "done") {
    actions.push(
      action(
        card.pinned ? "Unpin" : "Pin",
        card.pinned ? "pin-off" : "pin",
        () => M.pin(id),
        mobile ? {} : { kbd: "P" },
      ),
    );
  }
  actions.push(action("Fork", "git-fork", () => M.fork(id)));
  if (card.state === "ready") actions.push(action("Merge", "git-merge", () => queueMerge(card)));
  return actions;
}

const lowerFirst = (text: string): string => text.charAt(0).toLowerCase() + text.slice(1);

/** Items of the More actions menu: a move to every other column, then the fixed ones. */
export function moreItems(card: Card, close: () => void): MoreItem[] {
  const id = card.id;
  const run = (fn: () => void) => () => {
    close();
    fn();
  };
  const moves = M.COLUMNS.filter((col: Column) => col !== M.colOf(card.state)).map((col) => ({
    label: `Move to ${lowerFirst(M.STATUS[col].label)}`,
    icon: M.STATUS[col].icon,
    danger: false,
    run: run(() => M.moveCard(id, col)),
  }));
  return [
    ...moves,
    {
      label: "Restore a checkpoint",
      icon: "history",
      danger: false,
      run: run(() => M.setTab("activity")),
    },
    {
      label: "Simulate CI failure",
      icon: "circle-x",
      danger: false,
      run: run(() => M.simulateCiFailure(id)),
    },
    {
      label: "Copy branch name",
      icon: "copy",
      danger: false,
      run: run(() => {
        navigator.clipboard?.writeText(card.branch ?? "").catch(() => undefined);
        M.toast("Branch name copied");
      }),
    },
    { label: "Delete card", icon: "trash-2", danger: true, run: run(() => M.deleteCard(id)) },
  ];
}
