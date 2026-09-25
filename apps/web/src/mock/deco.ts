import { openCard } from "./actions/navigation";
import { thinkSupported } from "./agents";
import { type CardKey, cardLabel } from "./card-key";
import { CI, colOf, isAwake, STATUS, tone } from "./constants";
import type { Ctx } from "./context";
import { dragStart } from "./dom/drag";
import { full, money } from "./format";
import { person, proj, rel } from "./selectors";
import type { Card, Column, Status } from "./types";

interface Avatar {
  key: string;
  isAgent: boolean;
  isPerson: boolean;
  initials: string;
  title: string;
}

/** View model of a card, the prototype's `deco(c)`. */
export interface CardView {
  id: CardKey;
  key: string;
  num: string;
  title: string;
  p: string;
  projectName: string;
  state: Status;
  col: Column;
  stateLabel: string;
  icon: string;
  edge: string;
  iconColor: string;
  stColor: string;
  subtle: string;
  titleColor: string;
  role: string;
  agent: string;
  model: string;
  think: string;
  hasThink: boolean;
  perm: string;
  hasModel: boolean;
  showDoing: boolean;
  doing: string;
  paused: boolean;
  showReason: boolean;
  reason: string;
  branch: string | null;
  hasBranch: boolean;
  pkg: string | null;
  hasPkg: boolean;
  hasCi: boolean;
  ciLabel: string;
  ciIcon: string;
  ciColor: string;
  ciTip: string;
  cost: string;
  hasCost: boolean;
  costNum: number;
  asleep: boolean;
  waking: boolean;
  pinned: boolean;
  bypass: boolean;
  merging: boolean;
  mergePct: string;
  mergeNum: number;
  sleepLabel: string;
  awakeLabel: string;
  hasFooter: boolean;
  avatars: Avatar[];
  hasAvatars: boolean;
  clDone: number;
  clTotal: number;
  hasCl: boolean;
  clLabel: string;
  clTip: string;
  commentsN: number;
  hasComments: boolean;
  commentsTip: string;
  attachN: number;
  hasAttach: boolean;
  attachTip: string;
  aria: string;
  upd: string;
  updFull: string;
  updTs: number;
  labels: string[];
  selected: boolean;
  focused: boolean;
  dragging: boolean;
  ring: string;
  opacity: string;
  open: () => void;
  down: (e: PointerEvent) => void;
  key2: (e: KeyboardEvent) => void;
}

const LIVE_STATES: readonly Status[] = ["working", "merging", "planning"];

function statusBits(c: Card) {
  const st = STATUS[c.state];
  const t = st.tone;
  const quiet = c.state === "done" || c.asleep;
  return {
    state: c.state,
    col: colOf(c.state),
    stateLabel: st.label,
    icon: st.icon,
    edge: t ? tone(t, "solid") : "var(--color-border)",
    iconColor: t ? tone(t, "solid") : "var(--color-text-muted)",
    stColor: t ? tone(t, "text") : "var(--color-text-secondary)",
    subtle: tone(t, "subtle"),
    titleColor: quiet ? "var(--color-text-secondary)" : "var(--color-text-primary)",
    showDoing: LIVE_STATES.includes(c.state) && !!c.doing && !c.asleep && !c.paused,
    doing: c.doing,
    paused: c.paused && c.state === "working",
    showReason: c.state === "needs",
    reason: c.reason,
    merging: c.state === "merging",
    mergePct: `${c.mergePct}%`,
    mergeNum: c.mergePct,
  };
}

function agentBits(ctx: Ctx, c: Card) {
  const think = c.think && thinkSupported(ctx, c.model) ? c.think : "";
  const awakeLabel = c.asleep ? "Asleep" : isAwake(c) ? "Awake" : "No session";
  return {
    role: c.role,
    agent: c.agent,
    model: c.model,
    think,
    hasThink: !!think,
    perm: c.perm,
    hasModel: c.state !== "backlog",
    asleep: c.asleep,
    waking: c.waking,
    pinned: c.pinned,
    bypass: c.bypass,
    sleepLabel: c.waking ? "Waking" : "Asleep",
    awakeLabel: c.waking ? "Waking" : awakeLabel,
  };
}

function ciBits(c: Card) {
  const ci = c.ci ? CI[c.ci] : null;
  return {
    branch: c.branch,
    hasBranch: !!c.branch,
    pkg: c.pkg,
    hasPkg: !!c.pkg,
    hasCi: !!ci,
    ciLabel: ci ? ci.label : "",
    ciIcon: ci ? ci.icon : "",
    ciColor: ci ? ci.color : "",
    ciTip: ci ? `CI ${ci.label.toLowerCase()}` : "",
    cost: money(c.cost),
    hasCost: c.cost > 0,
    costNum: c.cost,
  };
}

const initialsOf = (name: string): string =>
  name
    .split(" ")
    .map((x) => x.charAt(0))
    .join("")
    .slice(0, 2);

function avatarsOf(ctx: Ctx, c: Card): Avatar[] {
  const people = c.members.flatMap((pid): Avatar[] => {
    const p = person(ctx, pid);
    if (!p) return [];
    return [
      { key: pid, isAgent: false, isPerson: true, initials: initialsOf(p.name), title: p.name },
    ];
  });
  if (c.state === "backlog") return people;
  return [
    ...people,
    { key: "agent", isAgent: true, isPerson: false, initials: "", title: `${c.agent} agent` },
  ];
}

const plural = (n: number, one: string, many: string): string => `${n}${n === 1 ? one : many}`;

function countBits(ctx: Ctx, c: Card) {
  const clDone = c.checklists.reduce((a, l) => a + l.items.filter((i) => i.done).length, 0);
  const clTotal = c.checklists.reduce((a, l) => a + l.items.length, 0);
  const commentsN = c.comments.length;
  const attachN = c.comments.reduce((a, x) => a + x.att.filter((t) => t.kind !== "link").length, 0);
  const avatars = avatarsOf(ctx, c);
  return {
    avatars,
    hasAvatars: avatars.length > 0,
    clDone,
    clTotal,
    hasCl: clTotal > 0,
    clLabel: `${clDone}/${clTotal}`,
    clTip: `${clDone} of ${clTotal} checklist items done`,
    commentsN,
    hasComments: commentsN > 0,
    commentsTip: plural(commentsN, " comment", " comments"),
    attachN,
    hasAttach: attachN > 0,
    attachTip: plural(attachN, " attachment", " attachments"),
    hasFooter:
      !!c.branch ||
      !!c.ci ||
      c.cost > 0 ||
      c.pinned ||
      c.checklists.length > 0 ||
      commentsN > 0 ||
      c.members.length > 0,
  };
}

function ariaOf(c: Card): string {
  const parts = [`${cardLabel(c)} ${c.title}. ${STATUS[c.state].label}`];
  if (c.state === "needs") parts.push(`: ${c.reason}`);
  if (c.asleep) parts.push(". Asleep");
  if (c.pinned) parts.push(". Pinned");
  if (c.bypass) parts.push(". Bypass permissions on");
  return parts.join("");
}

function selectionBits(ctx: Ctx, c: Card) {
  const { S } = ctx;
  const selected = S.openId === c.id;
  const focused = S.focusId === c.id;
  const dragging = S.dragId === c.id;
  let ring = "none";
  if (selected) ring = "0 0 0 2px var(--color-ink)";
  else if (focused) ring = "0 0 0 2px var(--color-border-strong)";
  return { selected, focused, dragging, ring, opacity: dragging ? "0.4" : "1" };
}

/** The card's view model. Views wrap it in a memo: `createMemo(() => M.deco(card))`. */
export function deco(ctx: Ctx, c: Card): CardView {
  return {
    id: c.id,
    // The card's key (`api#41`), so two cards numbered 12 in two projects have different keys.
    // The prototype used `c<number>`, which collided across projects; `mock/testing/proto-shape.ts`
    // translates this back to that shape for the differential tests.
    key: c.id,
    num: cardLabel(c),
    title: c.title,
    p: c.p,
    projectName: proj(ctx, c.p)?.name ?? "",
    ...statusBits(c),
    ...agentBits(ctx, c),
    ...ciBits(c),
    ...countBits(ctx, c),
    aria: ariaOf(c),
    upd: rel(ctx, c.upd),
    updFull: full(c.upd),
    updTs: c.upd,
    labels: c.labels,
    ...selectionBits(ctx, c),
    open: () => {
      if (!ctx.flags.suppressClick) openCard(ctx, c.id);
    },
    down: (e) => dragStart(ctx, e, c.id),
    key2: (e) => {
      if (e.key === "Enter") openCard(ctx, c.id);
    },
  };
}
