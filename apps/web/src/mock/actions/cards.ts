import { batch } from "solid-js";
import { defaultModel, thinkSupported } from "../agents";
import { type CardKey, cardKey, cardLabel, nextCardNumber } from "../card-key";
import { colOf, STATUS } from "../constants";
import type { Ctx } from "../context";
import { addAct, announce, confirm, later, pushMsg, toast } from "../engine";
import { checksFor } from "../seed/checks";
import { card, cardLabelOf } from "../selectors";
import type { State } from "../state-types";
import type { Card, Column, Status } from "../types";
import { openCard } from "./navigation";
import { startSession } from "./sessions";

/** Why a card cannot be dragged from one column to another, or null when it can. */
export function refuse(c: Card, from: Column, to: Column): string | null {
  if (from === "done") return "Done cards are merged. Fork the card to keep working on it.";
  if (to === "done") return "Cards move to Done by themselves after they merge.";
  if (to === "needs") {
    return "Cards move to Needs you by themselves when an agent is waiting on you.";
  }
  if (to === "review" && (!c.branch || from === "backlog" || from === "planning")) {
    return "In review needs an open pull request. The agent opens one when the work is ready.";
  }
  if (to === "ready" && from !== "review") {
    return "Ready to merge needs an approved review and passing checks.";
  }
  if (to === "ready" && c.ci !== "passed") {
    return "Checks haven't passed on this card yet, so it can't be ready to merge.";
  }
  if (c.state === "merging")
    return "The Integrator is merging this card. Wait for the merge to finish.";
  return null;
}

/** A refused card sits in the new column this long before it snaps back. */
const SNAP_BACK_MS = 420;
/** Cards moved to review without a pull request get a made-up number above this. */
const FAKE_PR_BASE = 300;

function applyMove(ctx: Ctx, c: Card, from: Column, to: Column): void {
  if (from === "backlog" && (to === "working" || to === "planning")) {
    startSession(ctx, c, to);
    toast(ctx, "Card started");
  } else if (to === "backlog") {
    c.paused = false;
    c.doing = "";
    c.asleep = true;
    toast(ctx, "Card moved to backlog. Session is asleep.");
  } else if (to === "working" && from === "review") {
    c.doing = "Addressing review comments";
    toast(ctx, "Sent back to working");
  } else if (to === "working" && from === "planning") {
    c.doing = "Starting without a plan";
    toast(ctx, "Plan skipped");
  } else if (to === "review") {
    c.pr = c.pr || FAKE_PR_BASE + c.n;
    c.ci = "running";
    c.doing = "";
    toast(ctx, "Pull request opened");
  } else if (to === "ready") {
    toast(ctx, "Ready to merge");
  }
}

/** Moves a card by hand. A refused move shows briefly, then snaps back with a toast. */
export function moveCard(ctx: Ctx, id: CardKey, to: Column): void {
  const c = card(ctx, id);
  if (!c) return;
  const from = colOf(c.state);
  if (from === to) return;
  const prev = { state: c.state, reason: c.reason };
  const why = refuse(c, from, to);
  c.state = to;
  if (why) {
    later(SNAP_BACK_MS, () => {
      Object.assign(c, prev);
      toast(ctx, why);
    });
    return;
  }
  c.upd = Date.now();
  c.asleep = false;
  applyMove(ctx, c, from, to);
  announce(ctx, `${cardLabelOf(ctx, c)} moved to ${STATUS[to].label}`);
}

const forkState = (s: Status): Status => (s === "backlog" ? "backlog" : "working");

/** Copies a card into a new one. Like the prototype, the copy shares the checklist and comment lists. */
export function fork(ctx: Ctx, id: CardKey): void {
  const { S } = ctx;
  const c = card(ctx, id);
  if (!c) return;
  const n = nextCardNumber(S.cards, c.p);
  const f: Card = {
    ...c,
    id: cardKey(c.p, n),
    n,
    title: `${c.title} (fork)`,
    state: forkState(c.state),
    branch: c.branch ? `${c.branch}-fork` : null,
    pr: null,
    ci: null,
    cost: 0,
    pinned: false,
    asleep: false,
    upd: Date.now(),
    doing: "Starting from the latest checkpoint",
    reason: "",
    mergePct: 0,
  };
  S.cards.push(f);
  S.chat[f.id] = [
    ctx.msg.system(
      `Forked from ${cardLabel(c)} at its latest checkpoint. This card has its own worktree and a copy of the session context.`,
    ),
  ];
  S.act[f.id] = [];
  S.checks[f.id] = checksFor(f);
  toast(ctx, "Card forked", { label: "Open", run: () => openCard(ctx, f.id) });
}

/**
 * Drops what the store keeps per card key. A new card can take the number of a deleted one, so
 * the deleted card must leave no chat, activity, checks, note, or preview state behind.
 */
function forgetCardData(S: State, id: CardKey): void {
  delete S.chat[id];
  delete S.act[id];
  delete S.checks[id];
  if (S.notes) delete S.notes[id];
  if (S.preview) delete S.preview[id];
}

export function deleteCard(ctx: Ctx, id: CardKey): void {
  const { S } = ctx;
  const c = card(ctx, id);
  if (!c) return;
  confirm(ctx, {
    title: "Delete card",
    message: c.branch
      ? `This deletes ${cardLabel(c)}, its session, and its worktree with unmerged work on ${c.branch}.`
      : `This deletes ${cardLabel(c)} and its notes.`,
    action: "Delete card",
    destructive: true,
    run: () =>
      batch(() => {
        S.cards = S.cards.filter((x) => x.id !== id);
        forgetCardData(S, id);
        if (S.openId === id) S.openId = null;
        toast(ctx, "Card deleted");
      }),
  });
}

export function rename(ctx: Ctx, id: CardKey, title: string): void {
  const c = card(ctx, id);
  if (c && title?.trim()) c.title = title.trim();
}

export type SettingKey = "agent" | "role" | "model" | "think" | "perm";

const SETTING_LABELS: Record<SettingKey, string> = {
  agent: "Agent",
  role: "Role",
  model: "Model",
  think: "Thinking mode",
  perm: "Permission mode",
};

/** Keeps thinking mode valid after the model changed: off when unsupported, Medium when newly supported. */
function fixThinking(ctx: Ctx, c: Card): void {
  if (!thinkSupported(ctx, c.model)) c.think = null;
  else if (!c.think) c.think = "Medium";
}

/** Changes one agent setting. Turning on bypass asks for confirmation first. */
export function setSetting(ctx: Ctx, id: CardKey, key: SettingKey, val: string): void {
  const c = card(ctx, id);
  if (!c) return;
  if (key === "perm" && val === "Bypass permissions" && !c.bypass) {
    requestBypass(ctx, id);
    return;
  }
  if (key === "perm") c.bypass = false;
  if (key === "agent") c.model = defaultModel(ctx, val) ?? c.model;
  c[key] = val;
  if (key === "model" || key === "agent") fixThinking(ctx, c);
  const label = SETTING_LABELS[key];
  if (c.state !== "backlog") {
    pushMsg(ctx, id, ctx.msg.system(`${label} set to ${val}. It takes effect on the next turn.`));
  }
  addAct(ctx, id, { kind: "tool", text: `${label} set to ${val}` });
}

export function requestBypass(ctx: Ctx, id: CardKey): void {
  const target = card(ctx, id);
  if (!target) return;
  confirm(ctx, {
    title: "Turn on bypass permissions",
    message: `The agent on ${cardLabel(target)} will run every command and edit without asking. It stays inside this card's worktree and every action is still audited.`,
    action: "Turn on bypass",
    destructive: true,
    ack: "I understand the agent can run any command in the worktree without asking.",
    run: () =>
      batch(() => {
        const c = card(ctx, id);
        if (!c) return;
        c.perm = "Bypass permissions";
        c.bypass = true;
        pushMsg(
          ctx,
          id,
          ctx.msg.system(
            "Bypass permissions is on for this card. The agent can run any command in its worktree without asking.",
          ),
        );
        addAct(ctx, id, { kind: "approval", text: "Bypass permissions turned on by you" });
        toast(ctx, "Bypass turned on");
      }),
  });
}

export function turnOffBypass(ctx: Ctx, id: CardKey): void {
  const c = card(ctx, id);
  if (!c) return;
  c.bypass = false;
  c.perm = "Full auto";
  pushMsg(ctx, id, ctx.msg.system("Bypass permissions is off. Permission mode set to Full auto."));
  toast(ctx, "Bypass turned off");
}
