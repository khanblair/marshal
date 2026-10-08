import { SessionStateStopped } from "@marshal/protocol";
import { batch } from "solid-js";
import { type ApprovalMsg, type Card, type Column, M, type PlanMsg } from "~/mock";
import type { CardKey } from "~/mock/card-key";
import { exportCardToDoc, exportKey, saveNoteToDrive } from "~/views/google-export/export-actions";
import { isExporting } from "~/views/google-export/export-run";

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
  /** True while the item's work is already running. */
  disabled?: boolean;
  /** Muted words after the label. */
  hint?: string;
}

type Pending = ApprovalMsg | PlanMsg | undefined;

const action = (
  label: string,
  icon: string,
  run: () => void,
  extra: Partial<CardAction> = {},
): CardAction => ({ label, icon, run, primary: false, disabled: false, ...extra });

function approvalAction(id: CardKey, pending: Pending): CardAction[] {
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
  // The agent stopped without being asked to, which is what put the card in Needs you.
  if (card.state === "needs" && card.session === SessionStateStopped) {
    return [action("Resume agent", "play", () => M.resume(card.id), { primary: lead })];
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

/**
 * The Simulate CI failure items (N28, B6.4). Three cases, asked as the two questions the daemon's own
 * rule answers: a card a daemon owns while that daemon runs in dev mode gets both of its modes, a card
 * a daemon owns on a daemon that is not in dev mode gets neither (the routes exist only in dev mode),
 * and a card the mock made keeps the mock's own one-item story. The real mode is the second item and
 * is labeled so that what it does is plain before it is pressed; it asks for confirmation too.
 */
function simulateItems(card: Card, run: (fn: () => void) => () => void): MoreItem[] {
  const mockItem: MoreItem = {
    label: "Simulate CI failure",
    icon: "circle-x",
    danger: false,
    run: run(() => M.simulateCiFailure(card.id)),
  };
  if (M.simulateOnDaemon(card)) {
    return [
      mockItem,
      {
        label: "Simulate CI failure on GitHub",
        icon: "github",
        danger: false,
        run: run(() => M.simulateCiFailureReal(card.id)),
      },
    ];
  }
  return M.ciOnDaemon(card) ? [] : [mockItem];
}

/** The three ways to move a card's words to and from Google. One that already runs shows as working. */
function googleItems(
  card: Card,
  run: (fn: () => void) => () => void,
  openImport: () => void,
): MoreItem[] {
  const work = (label: string, icon: string, key: string, fn: () => void): MoreItem => {
    const busy = isExporting(key);
    return {
      label,
      icon,
      danger: false,
      run: run(fn),
      disabled: busy,
      hint: busy ? "Working…" : undefined,
    };
  };
  return [
    work("Export to Google Doc", "file-text", exportKey("doc", card.id), () => {
      void exportCardToDoc(card);
    }),
    work("Save note to Google Drive", "upload", exportKey("note", card.id), () => {
      void saveNoteToDrive(card);
    }),
    { label: "Import from Google link…", icon: "link", danger: false, run: run(openImport) },
  ];
}

/**
 * Items of the More actions menu: a move to every other column, then the Google ones, then the
 * fixed ones. `openImport` opens the Import from Google link dialog.
 */
export function moreItems(
  card: Card,
  close: () => void,
  openImport: () => void = () => undefined,
): MoreItem[] {
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
    ...googleItems(card, run, openImport),
    {
      label: "Restore a checkpoint",
      icon: "history",
      danger: false,
      run: run(() => M.setTab("activity")),
    },
    ...simulateItems(card, run),
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
