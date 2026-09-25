import { batch } from "solid-js";
import { createMutable } from "solid-js/store";
import { STATUS } from "./constants";
import type { Ctx } from "./context";
import { takeMid } from "./ids";
import type { State } from "./state-types";
import type {
  Activity,
  ActivityKind,
  ActivityState,
  AgentMsg,
  Card,
  DialogSpec,
  FeedItem,
  InfoNotice,
  Msg,
  Notice,
  Status,
  ToastAction,
} from "./types";

/**
 * The store proxy of an object that is, or is about to be, in the store. Code that
 * keeps a reference and writes to it later must hold the proxy: writes to the raw
 * object change the data without telling any view.
 */
export const live = <T extends object>(value: T): T => createMutable(value);

/** Runs `fn` after `ms`, applying all its writes at once like one prototype `emit()`. */
export function later(ms: number, fn: () => void): void {
  setTimeout(() => batch(fn), ms);
}

export interface Step {
  afterMs: number;
  run: () => void;
}

/** Runs the steps one after another, each `afterMs` after the previous one. */
export function seq(steps: readonly Step[]): void {
  let at = 0;
  for (const step of steps) {
    at += step.afterMs;
    later(at, step.run);
  }
}

/** Kept for the prototype's call sites. The store notifies views by itself, so there is nothing to flush. */
export function emit(): void {}

export type StatePatch = Partial<State> | ((S: State) => Partial<State>);

export function set(ctx: Ctx, patch: StatePatch): void {
  const values = typeof patch === "function" ? patch(ctx.S) : patch;
  batch(() => {
    Object.assign(ctx.S, values);
  });
}

export function announce(ctx: Ctx, text: string): void {
  ctx.S.announce = text;
}

const TOAST_MS = 4000;
const TOAST_WITH_ACTION_MS = 8000;
const MAX_TOASTS = 3;

export function toast(ctx: Ctx, msg: string, action?: ToastAction): void {
  const { S } = ctx;
  const id = `t${takeMid(ctx.ids)}`;
  const dismiss = (): void => set(ctx, { toasts: S.toasts.filter((t) => t.id !== id) });
  S.toasts = [...S.toasts, { id, msg, action, dismiss }].slice(-MAX_TOASTS);
  setTimeout(dismiss, action ? TOAST_WITH_ACTION_MS : TOAST_MS);
}

export function confirm(ctx: Ctx, spec: DialogSpec): void {
  set(ctx, { dialog: { acked: false, ...spec } });
}

export function closeDialog(ctx: Ctx): void {
  set(ctx, { dialog: null });
}

/** The card's chat, created on first use. */
function chatList(ctx: Ctx, id: number): Msg[] {
  const list = ctx.S.chat[id];
  if (list) return list;
  const fresh = live<Msg[]>([]);
  ctx.S.chat[id] = fresh;
  return fresh;
}

function actList(ctx: Ctx, id: number): Activity[] {
  const list = ctx.S.act[id];
  if (list) return list;
  const fresh = live<Activity[]>([]);
  ctx.S.act[id] = fresh;
  return fresh;
}

/** Appends a message to a card's chat and returns its live proxy. */
export function pushMsg<T extends Msg>(ctx: Ctx, id: number, msg: T): T {
  const item = live(msg);
  chatList(ctx, id).push(item);
  return item;
}

const ACTIVITY_LIMIT = 200;

export interface ActInput {
  kind: ActivityKind;
  text: string;
  result?: string;
  st?: ActivityState;
}

/** Adds an entry to the top of a card's activity log and returns its live proxy. */
export function addAct(ctx: Ctx, id: number, input: ActInput): Activity {
  const item = live<Activity>({
    id: `a${takeMid(ctx.ids)}`,
    kind: input.kind,
    text: input.text,
    result: input.result || "",
    st: input.st || "ok",
    ts: Date.now(),
    fresh: true,
  });
  const list = actList(ctx, id);
  list.unshift(item);
  ctx.S.act[id] = list.slice(0, ACTIVITY_LIMIT);
  return item;
}

/** Moves a card to a state. Leaving "needs" clears the reason unless `extra` sets one. */
export function setState(ctx: Ctx, c: Card, st: Status, extra?: Partial<Card>): void {
  const from = c.state;
  Object.assign(c, { state: st, upd: Date.now() }, extra);
  if (st !== "needs") c.reason = extra?.reason || "";
  if (from !== st) announce(ctx, `#${c.id} moved to ${STATUS[st].label}`);
}

const STREAM_STEP_MS = 45;
/** `split(/(\s+)/)` keeps the spaces as tokens, so 3 tokens are about 2 words. */
const STREAM_TOKENS_PER_STEP = 3;

/** Types an agent message into `list` a few words at a time, then calls `done`. */
export function stream(ctx: Ctx, list: Msg[], text: string, done?: () => void): AgentMsg {
  const msg = live(ctx.msg.streaming());
  list.push(msg);
  const tokens = text.split(/(\s+)/);
  let i = 0;
  const timer = setInterval(() => {
    batch(() => {
      msg.text += tokens.slice(i, i + STREAM_TOKENS_PER_STEP).join("");
      i += STREAM_TOKENS_PER_STEP;
      if (i < tokens.length) return;
      msg.streaming = false;
      clearInterval(timer);
      done?.();
    });
  }, STREAM_STEP_MS);
  return msg;
}

export const streamCard = (ctx: Ctx, id: number, text: string, done?: () => void): AgentMsg =>
  stream(ctx, chatList(ctx, id), text, done);

export interface ToolRun {
  icon: string;
  action: string;
  result: string;
  ms: number;
  done?: () => void;
  kind?: ActivityKind;
}

/** Shows a running tool call in chat and activity, then completes it after `ms`. */
export function runTool(ctx: Ctx, id: number, run: ToolRun): void {
  const msg = pushMsg(ctx, id, ctx.msg.tool(run.icon, run.action, "Running", { st: "running" }));
  const kind = run.kind || (run.icon === "terminal" ? "command" : "file");
  const act = addAct(ctx, id, { kind, text: run.action, result: "Running", st: "running" });
  later(run.ms, () => {
    msg.st = "ok";
    msg.result = run.result;
    act.st = "ok";
    act.result = run.result;
    run.done?.();
  });
}

export function notice(ctx: Ctx, input: Omit<InfoNotice, "id" | "ts">): void {
  const item: Notice = { id: `n${takeMid(ctx.ids)}`, ts: Date.now(), ...input };
  ctx.S.notices = [item, ...ctx.S.notices];
}

const FEED_LIMIT = 80;

export function feed(ctx: Ctx, item: Omit<FeedItem, "id" | "ts">): void {
  const { S } = ctx;
  S.feed.unshift({ id: `f${takeMid(ctx.ids)}`, ...item, ts: Date.now() });
  S.feed = S.feed.slice(0, FEED_LIMIT);
}
