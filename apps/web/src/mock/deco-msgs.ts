import { approve, approvePlan, deny, editPlan, rejectPlan, savePlan } from "./actions/approvals";
import { openCard } from "./actions/navigation";
import { type CardKey, cardLabel } from "./card-key";
import { tone } from "./constants";
import type { Ctx } from "./context";
import { type CardView, deco } from "./deco";
import { formValue } from "./dom/form";
import { card } from "./selectors";
import type { ApprovalMsg, DiffMsg, Msg, PlanMsg, ToolMsg } from "./types";

interface MsgBase {
  id: string;
  key: string;
  isUser: boolean;
  isAgent: boolean;
  isTool: boolean;
  isPlan: boolean;
  isApproval: boolean;
  isDiff: boolean;
  isSystem: boolean;
  isCard: boolean;
  isLinks: boolean;
  text: string;
  streaming: boolean;
}

interface ToolView {
  verb: string;
  target: string;
  mono: boolean;
  icon: string;
  result: string;
  stIcon: string;
  resColor: string;
  open: boolean;
  hasDetail: boolean;
  noDetail: boolean;
  detail: string;
  chev: string;
  toggle: () => void;
  targetFont: string;
  targetSize: string;
}

interface DiffView {
  summary: string;
  add: string;
  del: string;
  openDiff: () => void;
}

interface PlanView {
  steps: { n: string; t: string }[];
  files: string[];
  risks: string[];
  checks: string[];
  waiting: boolean;
  editing: boolean;
  notEditing: boolean;
  done: boolean;
  statusLabel: string;
  statusIcon: string;
  statusColor: string;
  statusBg: string;
  editText: string;
  approve: () => void;
  reject: () => void;
  edit: () => void;
  cancel: () => void;
  save: (e: SubmitEvent) => void;
}

interface ApprovalView {
  cmd: string;
  why: string;
  waiting: boolean;
  done: boolean;
  resultLabel: string;
  resultIcon: string;
  resultColor: string;
  hasCard: boolean;
  cardLabel: string;
  openCard: () => void;
  approve: () => void;
  deny: () => void;
  keys: (e: KeyboardEvent) => void;
}

/**
 * View model of one chat message, the prototype's `decoMsgs` item. The kind flags say
 * which of the optional groups is filled in.
 */
export type MsgView = MsgBase &
  Partial<ToolView> &
  Partial<DiffView> &
  Partial<PlanView> &
  Partial<ApprovalView> & { c?: CardView; cards?: CardView[] };

/** Tool targets that look like paths, files, or commands are set in the mono font. */
const MONO_TARGET = /[/@]|\.[a-z]{1,4}\b|^(go|pnpm|git|npm|k6|\.\/gradlew)\b/;

function toolView(x: ToolMsg): ToolView {
  const sp = x.action.indexOf(" ");
  const verb = sp > 0 ? x.action.slice(0, sp) : x.action;
  const target = sp > 0 ? x.action.slice(sp + 1) : "";
  const mono = MONO_TARGET.test(target) && !/^"/.test(target);
  let resColor = tone("working", "text");
  if (x.st === "fail") resColor = tone("danger", "text");
  else if (x.st === "running") resColor = "var(--color-text-secondary)";
  let stIcon = "check";
  if (x.st === "running") stIcon = "spinner";
  else if (x.st === "fail") stIcon = "x";
  return {
    verb,
    target,
    mono,
    icon: x.icon,
    result: x.result,
    stIcon,
    resColor,
    open: x.open,
    hasDetail: !!x.detail,
    noDetail: !x.detail,
    detail: x.detail,
    chev: x.open ? "chevron-down" : "chevron-right",
    toggle: () => {
      x.open = !x.open;
    },
    targetFont: mono ? "var(--font-mono)" : "var(--font-sans)",
    targetSize: mono ? "12px" : "14px",
  };
}

const diffView = (ctx: Ctx, x: DiffMsg, cardId: CardKey | null): DiffView => ({
  summary: `Changed ${x.files} files`,
  add: `+${x.add}`,
  del: `−${x.del}`,
  openDiff: () => {
    if (cardId !== null) openCard(ctx, cardId, "diff");
  },
});

function planStatus(x: PlanMsg) {
  if (x.st === "waiting") {
    return {
      statusLabel: x.edited ? "Edited, waiting for you" : "Waiting for you",
      statusIcon: "st-needs",
      statusColor: tone("needs-you", "text"),
      statusBg: tone("needs-you", "subtle"),
    };
  }
  const ok = x.st === "approved";
  return {
    statusLabel: ok ? "Approved" : "Rejected",
    statusIcon: ok ? "check" : "x",
    statusColor: ok ? tone("working", "text") : tone("danger", "text"),
    statusBg: ok ? tone("working", "subtle") : tone("danger", "subtle"),
  };
}

function planView(ctx: Ctx, x: PlanMsg, cardId: CardKey | null): PlanView {
  const waiting = x.st === "waiting";
  // A project chat has no card behind it; its actions then find no card and do nothing.
  const id = cardId ?? "";
  return {
    steps: x.steps.map((t, i) => ({ n: `${i + 1}.`, t })),
    files: x.files,
    risks: x.risks,
    checks: x.checks,
    waiting: waiting && !x.editing,
    editing: waiting && x.editing,
    notEditing: !(waiting && x.editing),
    done: !waiting,
    ...planStatus(x),
    editText: x.steps.join("\n"),
    approve: () => approvePlan(ctx, id),
    reject: () => rejectPlan(ctx, id),
    edit: () => editPlan(ctx, id, true),
    cancel: () => editPlan(ctx, id, false),
    save: (e) => {
      e.preventDefault();
      savePlan(ctx, id, formValue(e, "steps"));
    },
  };
}

function approvalKeys(ctx: Ctx, id: CardKey, waiting: boolean) {
  return (e: KeyboardEvent): void => {
    if (!waiting) return;
    if (e.key === "Enter" && e.target === e.currentTarget) {
      e.preventDefault();
      approve(ctx, id);
    }
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      deny(ctx, id);
    }
  };
}

function approvalView(ctx: Ctx, x: ApprovalMsg, cardId: CardKey | null): ApprovalView {
  const id = x.cardId || cardId || "";
  const waiting = x.st === "waiting";
  const ok = x.st === "approved";
  const ref = x.cardId ? card(ctx, x.cardId) : undefined;
  return {
    cmd: x.cmd,
    why: x.why,
    waiting,
    done: !waiting,
    resultLabel: ok ? "Approved" : "Denied",
    resultIcon: ok ? "check" : "x",
    resultColor: ok ? tone("working", "text") : tone("danger", "text"),
    hasCard: !!x.cardId,
    cardLabel: ref ? `${cardLabel(ref)} ${ref.title}` : "",
    openCard: () => openCard(ctx, id),
    approve: () => approve(ctx, id),
    deny: () => deny(ctx, id),
    keys: approvalKeys(ctx, id, waiting),
  };
}

function baseView(x: Msg): MsgBase {
  return {
    id: x.id,
    key: x.id,
    isUser: x.k === "user",
    isAgent: x.k === "agent",
    isTool: x.k === "tool",
    isPlan: x.k === "plan",
    isApproval: x.k === "approval",
    isDiff: x.k === "diff",
    isSystem: x.k === "system",
    isCard: x.k === "card",
    isLinks: x.k === "links",
    text: ("text" in x && x.text) || "",
    streaming: x.k === "agent" && !!x.streaming,
  };
}

function msgView(ctx: Ctx, x: Msg, cardId: CardKey | null): MsgView {
  const base = baseView(x);
  switch (x.k) {
    case "tool":
      return { ...base, ...toolView(x) };
    case "diff":
      return { ...base, ...diffView(ctx, x, cardId) };
    case "plan":
      return { ...base, ...planView(ctx, x, cardId) };
    case "approval":
      return { ...base, ...approvalView(ctx, x, cardId) };
    case "card": {
      const c = card(ctx, x.cardId);
      return c ? { ...base, c: deco(ctx, c) } : { ...base, isCard: false };
    }
    case "links":
      return {
        ...base,
        cards: x.cards.flatMap((id) => {
          const c = card(ctx, id);
          return c ? [deco(ctx, c)] : [];
        }),
      };
    default:
      return base;
  }
}

/** View models for a chat. `cardId` is the card whose chat this is, or null for project chats. */
export const decoMsgs = (ctx: Ctx, list: readonly Msg[], cardId: CardKey | null): MsgView[] =>
  list.map((x) => msgView(ctx, x, cardId));
