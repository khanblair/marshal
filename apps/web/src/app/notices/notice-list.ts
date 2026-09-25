import { batch } from "solid-js";
import type { Card, InfoNotice, Marshal, Notice, SleepNotice } from "~/mock";
import { type CardKey, cardLabel } from "~/mock/card-key";

/* View model of the notices panel, ported from the notices part of the design's `renderVals`. */

/** Icon color of a notice. */
export type NoticeTone = "needs" | "secondary" | "danger";

export interface NoticeActionModel {
  label: string;
  run: () => void;
  /** Ink fill instead of the outlined surface button. */
  primary: boolean;
}

export interface NoticeRowModel {
  /** The card's label and title, such as `#41 Fix token refresh`. */
  title: string;
  /** The card's project, shown under the title so cards of two projects with the same number differ. */
  project: string;
  sub: string;
  subTone: "needs" | "secondary";
  open: () => void;
  actions: NoticeActionModel[];
}

export interface NoticeModel {
  key: string;
  icon: string;
  tone: NoticeTone;
  title: string;
  sub: string;
  when: string;
  full: string;
  /** The project of the card a notice is about, shown beside the time, so two projects with the same card number differ. */
  project?: string;
  /** Set when the notice has a Dismiss button. */
  dismiss?: (() => void) | undefined;
  rows: NoticeRowModel[];
  actions: NoticeActionModel[];
}

const MS_PER_SECOND = 1000;
const SECONDS_PER_MINUTE = 60;
const SECONDS_DIGITS = 2;

const SLEEP_SUB =
  "Sleeping frees memory and keeps each session. Cards wake in a few seconds when needed.";

type CardOpener = (id: CardKey) => () => void;

const act = (label: string, run: () => void, primary = false): NoticeActionModel => ({
  label,
  run,
  primary,
});

/** Time left before the idle cards sleep, as `m:ss`. */
export function countdown(deadline: number, now: number): string {
  const left = Math.max(0, Math.ceil((deadline - now) / MS_PER_SECOND));
  const minutes = Math.floor(left / SECONDS_PER_MINUTE);
  const seconds = String(left % SECONDS_PER_MINUTE).padStart(SECONDS_DIGITS, "0");
  return `${minutes}:${seconds}`;
}

/** Closes the panel and opens the card. On phones it first goes to the card's board. */
function cardOpener(M: Marshal, phone: boolean): CardOpener {
  return (id) => () =>
    batch(() => {
      M.set({ noticesOpen: false });
      const c = M.card(id);
      if (phone && M.S.route.page !== "project" && c) M.go("project", c.p, "board");
      M.openCard(id);
    });
}

function needsRow(M: Marshal, c: Card, open: CardOpener): NoticeRowModel {
  const pending = M.pendingApproval(c.id);
  const actions = [act("Open", open(c.id))];
  if (pending?.k === "approval") actions.unshift(act("Approve", () => M.approve(c.id), true));
  if (pending?.k === "plan") actions.unshift(act("Review plan", open(c.id), true));
  return {
    title: `${cardLabel(c)} ${c.title}`,
    project: M.proj(c.p)?.name ?? "",
    sub: c.reason,
    subTone: "needs",
    open: open(c.id),
    actions,
  };
}

/** The group of every card waiting on you, oldest first. Absent when none waits. */
function needsNotice(M: Marshal, open: CardOpener): NoticeModel | null {
  const cards = M.needs();
  const oldest = cards[0];
  if (!oldest) return null;
  const projects = new Set(cards.map((c) => c.p)).size;
  return {
    key: "needs",
    icon: "st-needs",
    tone: "needs",
    title: `${cards.length}${cards.length === 1 ? " card needs you" : " cards need you"}`,
    sub: `Across ${projects} projects`,
    when: `Oldest ${M.rel(oldest.upd).toLowerCase()}`,
    full: M.full(oldest.upd),
    rows: cards.map((c) => needsRow(M, c, open)),
    actions: [],
  };
}

function base(M: Marshal, n: Notice) {
  return {
    key: n.id,
    when: M.rel(n.ts),
    full: M.full(n.ts),
    dismiss: () => M.dismissNotice(n.id),
  };
}

/** Skips a card that no longer exists, where the design would throw. */
function sleepRows(M: Marshal, id: CardKey, open: CardOpener): NoticeRowModel[] {
  const c = M.card(id);
  if (!c) return [];
  return [
    {
      title: `${cardLabel(c)} ${c.title}`,
      project: M.proj(c.p)?.name ?? "",
      sub: "",
      subTone: "secondary",
      open: open(id),
      actions: [
        act("Keep awake", () => M.keepAwake(id)),
        act("Sleep now", () => M.sleep(id)),
        act("Pin", () => M.pin(id)),
      ],
    },
  ];
}

function sleepNotice(M: Marshal, n: SleepNotice, open: CardOpener): NoticeModel {
  const count = n.cards.length;
  const lead =
    count === 1 ? " card is idle and will sleep in " : " cards are idle and will sleep in ";
  return {
    ...base(M, n),
    icon: "moon",
    tone: "secondary",
    title: `${count}${lead}${countdown(n.deadline, M.now())}`,
    sub: SLEEP_SUB,
    dismiss: undefined,
    rows: n.cards.flatMap((id) => sleepRows(M, id, open)),
    actions: [
      act("Keep all awake", () => M.keepAllAwake()),
      act("Sleep all now", () => M.sleepAll()),
    ],
  };
}

function openLimits(M: Marshal): void {
  batch(() => {
    M.S.settingsSection = "limits";
    M.S.noticesOpen = false;
    M.go("settings");
  });
}

function costNotice(M: Marshal, n: InfoNotice): NoticeModel {
  return {
    ...base(M, n),
    icon: "circle-dollar-sign",
    tone: "needs",
    title: n.text,
    sub: n.sub,
    rows: [],
    actions: [act("Open limits", () => openLimits(M))],
  };
}

function infoIcon(n: InfoNotice, danger: boolean): string {
  if (danger) return "circle-x";
  return n.kind === "plan" ? "st-needs" : "bell";
}

/** The project name of a plan or CI notice. The texts of the others (main failing, cost) already name it. */
function noticeProject(M: Marshal, n: InfoNotice): string {
  if (n.kind !== "plan" && n.kind !== "ci") return "";
  const pid = (n.cardId && M.card(n.cardId)?.p) || n.pid;
  return (pid && M.proj(pid)?.name) || "";
}

/** CI failures, main failing, and plans ready: an optional button that opens the card. */
function infoNotice(M: Marshal, n: InfoNotice, open: CardOpener): NoticeModel {
  const danger = n.kind === "ci" || n.kind === "ci-main";
  const target = n.cardId && M.card(n.cardId) ? n.cardId : null;
  const label = n.kind === "plan" ? "Review plan" : "Open card";
  return {
    ...base(M, n),
    icon: infoIcon(n, danger),
    tone: danger ? "danger" : "needs",
    title: n.text,
    sub: n.sub,
    project: noticeProject(M, n),
    rows: [],
    actions: target === null ? [] : [act(label, open(target), true)],
  };
}

function storeNotice(M: Marshal, n: Notice, open: CardOpener): NoticeModel {
  if (n.kind === "sleep") return sleepNotice(M, n, open);
  if (n.kind === "cost") return costNotice(M, n);
  return infoNotice(M, n, open);
}

/** Everything the notices panel lists, in the design's order: the needs-you group first. */
export function buildNotices(M: Marshal, phone: boolean): NoticeModel[] {
  const open = cardOpener(M, phone);
  const list: NoticeModel[] = [];
  const needs = needsNotice(M, open);
  if (needs) list.push(needs);
  for (const n of M.S.notices) list.push(storeNotice(M, n, open));
  return list;
}
