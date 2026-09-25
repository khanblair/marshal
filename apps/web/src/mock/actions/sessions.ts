import type { CardKey } from "../card-key";
import { isAwake } from "../constants";
import type { Ctx } from "../context";
import { addAct, announce, later, pushMsg, runTool, toast } from "../engine";
import { firstFile } from "../seed/files";
import { card, cardLabelOf } from "../selectors";
import type { Card, SleepNotice } from "../types";

const BRANCH_SLUG_MAX = 28;
/** A new session reads its first file shortly after the worktree is ready. */
const FIRST_READ_DELAY_MS = 1200;
const FIRST_READ_MS = 1500;
const WAKE_MS = 1600;

const slug = (title: string): string =>
  title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, BRANCH_SLUG_MAX);

/** Starts an agent session on a card. `c` must be the live card from the store. */
export function startSession(ctx: Ctx, c: Card, to?: "working" | "planning"): void {
  c.state = to || (c.perm === "Plan only" ? "planning" : "working");
  c.branch = c.branch || `marshal/${c.n}-${slug(c.title)}`;
  c.asleep = false;
  c.paused = false;
  c.doing =
    c.state === "planning" ? "Reading the code before writing a plan" : "Setting up the worktree";
  pushMsg(
    ctx,
    c.id,
    ctx.msg.system(`Session started with ${c.agent} in a new worktree on ${c.branch}.`),
  );
  addAct(ctx, c.id, { kind: "tool", text: `Created worktree on ${c.branch}` });
  later(FIRST_READ_DELAY_MS, () =>
    runTool(ctx, c.id, {
      icon: "file-search",
      action: `Read ${firstFile(c)}`,
      result: "120 lines",
      ms: FIRST_READ_MS,
      done: () => {
        if (c.state === "working") c.doing = `Editing ${firstFile(c)}`;
      },
    }),
  );
}

export function start(ctx: Ctx, id: CardKey): void {
  const c = card(ctx, id);
  if (!c) return;
  if (c.state === "backlog") {
    startSession(ctx, c);
    toast(ctx, "Card started");
    return;
  }
  if (c.paused) {
    c.paused = false;
    toast(ctx, "Card resumed");
    return;
  }
  if (c.asleep) wake(ctx, id);
}

export function pause(ctx: Ctx, id: CardKey): void {
  const c = card(ctx, id);
  if (!c) return;
  if (c.state !== "working") {
    toast(ctx, "Only working cards can be paused.");
    return;
  }
  c.paused = true;
  addAct(ctx, id, { kind: "tool", text: "Paused by you" });
  toast(ctx, "Card paused");
}

const sleepNotice = (ctx: Ctx): SleepNotice | undefined =>
  ctx.S.notices.find((n): n is SleepNotice => n.kind === "sleep");

/** Takes a card off the pending sleep notice, and drops the notice once it is empty. */
function dropFromSleep(ctx: Ctx, id: CardKey): void {
  const n = sleepNotice(ctx);
  if (!n) return;
  n.cards = n.cards.filter((x) => x !== id);
  if (!n.cards.length) ctx.S.notices = ctx.S.notices.filter((x) => x !== n);
}

export function sleep(ctx: Ctx, id: CardKey): void {
  const c = card(ctx, id);
  if (!c) return;
  if (c.state === "working" && !c.paused) {
    toast(ctx, "Working cards don't sleep. Pause the card first.");
    return;
  }
  if (c.state === "needs") {
    toast(ctx, "This card is waiting on you, so it stays awake.");
    return;
  }
  if (!isAwake(c)) {
    toast(ctx, "This card has no awake session.");
    return;
  }
  c.asleep = true;
  dropFromSleep(ctx, id);
  toast(ctx, "Card asleep");
  announce(ctx, `${cardLabelOf(ctx, c)} is asleep`);
}

export function wake(ctx: Ctx, id: CardKey): void {
  const c = card(ctx, id);
  if (!c?.asleep) return;
  c.waking = true;
  later(WAKE_MS, () => {
    c.waking = false;
    c.asleep = false;
    addAct(ctx, id, { kind: "tool", text: "Session resumed" });
    toast(ctx, "Session resumed");
  });
}

export function pin(ctx: Ctx, id: CardKey): void {
  const c = card(ctx, id);
  if (!c) return;
  c.pinned = !c.pinned;
  if (c.pinned) dropFromSleep(ctx, id);
  toast(ctx, c.pinned ? "Card pinned. It won't sleep." : "Card unpinned");
}

export function keepAwake(ctx: Ctx, id: CardKey): void {
  dropFromSleep(ctx, id);
  toast(ctx, "Kept awake for 15 more minutes");
}

export function sleepAll(ctx: Ctx): void {
  const n = sleepNotice(ctx);
  if (!n) return;
  for (const id of n.cards) {
    const c = card(ctx, id);
    if (c) c.asleep = true;
  }
  ctx.S.notices = ctx.S.notices.filter((x) => x !== n);
  toast(ctx, `${n.cards.length} cards asleep`);
}

export function keepAllAwake(ctx: Ctx): void {
  const n = sleepNotice(ctx);
  if (!n) return;
  ctx.S.notices = ctx.S.notices.filter((x) => x !== n);
  toast(ctx, `Kept ${n.cards.length} cards awake`);
}

export function dismissNotice(ctx: Ctx, id: string): void {
  ctx.S.notices = ctx.S.notices.filter((n) => n.id !== id);
}
