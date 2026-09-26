import {
  type AgentKind,
  type CardState,
  type CardViewMode,
  type CIState,
  type PermissionMode,
  type SessionState,
  SessionStateAsleep,
  SessionStateAwake,
  SessionStateSleepWarning,
  SessionStateStarting,
  SessionStateWaitingApproval,
  SessionStateWaking,
  SessionStateWorking,
  type ThinkingMode,
  type Card as WireCard,
} from "@marshal/protocol";
import { toMillis } from "./time";

/** A day in milliseconds, for the day numbers the Timeline and the calendar draw. */
const DAY_MS = 86_400_000;

/*
 * A card, from the daemon to the screens and back.
 *
 * The two sides do not speak the same language about four things, and this module is the one place
 * that knows both:
 *
 *   - The daemon names a card by an opaque id for routes and topics, and by its key
 *     (`api#41`) for people. The store is keyed by the key, so the key is the card's id here.
 *   - The daemon sends `claude`, `auto-edits`, and `high`. The screens show "Claude Code",
 *     "Auto-accept edits", and "High".
 *   - The daemon has timestamps; the Timeline and the calendar draw day numbers relative to today.
 *   - The daemon has no dependencies, members, checklists, comments, or cost yet (their own
 *     phases), so a real card carries none, and the parts of a card that own them show their empty
 *     states.
 */

/**
 * How much of the agent's context window a card has used, in percent, which is what the daemon
 * sends. The screens draw a fraction of the window, so a card's `ctx` is that figure over this.
 */
const PERCENT = 100;

/** The agent kinds the daemon sends and the names the screens show. */
const AGENT_NAMES: Record<AgentKind, string> = {
  claude: "Claude Code",
  gemini: "Gemini CLI",
  codex: "Codex",
  builtin: "Built-in agent",
};

/** The opposite direction, for a setting the screens changed. */
const AGENT_KINDS: Record<string, AgentKind> = {
  "Claude Code": "claude",
  "Gemini CLI": "gemini",
  Codex: "codex",
  "Built-in agent": "builtin",
};

/** The permission modes, in the words the screens use. */
const PERM_NAMES: Record<PermissionMode, string> = {
  ask: "Ask",
  "auto-edits": "Auto-accept edits",
  plan: "Plan only",
  "full-auto": "Full auto",
  bypass: "Bypass permissions",
};

const PERM_MODES: Record<string, PermissionMode> = {
  Ask: "ask",
  "Auto-accept edits": "auto-edits",
  "Plan only": "plan",
  "Full auto": "full-auto",
  "Bypass permissions": "bypass",
};

/** The thinking modes, in the words the screens use. */
const THINK_NAMES: Record<ThinkingMode, string> = {
  low: "Low",
  medium: "Medium",
  high: "High",
  "extra-high": "Extra high",
};

const THINK_MODES: Record<string, ThinkingMode> = {
  Low: "low",
  Medium: "medium",
  High: "high",
  "Extra high": "extra-high",
};

/**
 * The day number a moment falls on, counted from the start of the day the daemon is on. The
 * prototype's Timeline and calendar draw day numbers, not dates, and a card's dates come from the
 * daemon as moments.
 */
function dayOf(from: number, at: number | null | undefined): number | null {
  if (!at) return null;
  const start = new Date(from);
  start.setHours(0, 0, 0, 0);
  return Math.round((at - start.getTime()) / DAY_MS);
}

/**
 * What a stored session state means for the card that owns it (section S7c). The card panel, the
 * board, and the Agents view draw one flag for "asleep" and one for "waking", and a card that is
 * being brought back is drawn asleep with a spinner, the way the prototype did, so `asleep` is true
 * for both states and `waking` for the second alone. Every other state, and a card with no session,
 * is neither.
 */
export function sleepFlags(session: SessionState | null | undefined): {
  asleep: boolean;
  waking: boolean;
} {
  return {
    asleep: session === SessionStateAsleep || session === SessionStateWaking,
    waking: session === SessionStateWaking,
  };
}

/**
 * The session states in which a card has a running agent: it is on a turn, it waits for a message,
 * for an approval, or for the idle timer's sleep, or it is being started. Asleep and stopped
 * sessions have no process, a card that never had a session has none either, and a session that is
 * waking has none yet (it becomes awake when the resume has finished), so none of them counts.
 *
 * This is what the daemon's own count of awake cards (`badges.awake`, the live sessions of a
 * project's cards) counts. Home's list of awake cards on the daemon reads stored rows in the
 * awake, working, and waking states instead, and the fixture seeds rows that have no process; those
 * are the daemon's to reconcile, and the screens follow the live sessions here.
 */
const LIVE_SESSIONS: ReadonlySet<SessionState> = new Set<SessionState>([
  SessionStateStarting,
  SessionStateAwake,
  SessionStateWorking,
  SessionStateWaitingApproval,
  SessionStateSleepWarning,
]);

/** True when the state is one in which a card has a running agent. */
export const hasLiveSession = (session: SessionState | null | undefined): boolean =>
  session !== null && session !== undefined && LIVE_SESSIONS.has(session);

/**
 * A card the daemon fills, in the words the screens use for the same things. It is not the mock's
 * `Card`: that type also carries cost, dependencies, members, checklists, comments, the merge
 * percent, and the bypass flag, which the daemon does not speak about yet (their own phases). The
 * sync layer joins the two, which is why this type names only what the daemon owns.
 */
export interface DaemonCard {
  /** The card's key, `<projectId>#<number>`, which is how every screen and map names it. */
  id: string;
  /** The daemon's own id for the card, which its routes take. It is not the key. */
  daemonId: string;
  /** The number within its project, which is what `#41` shows. */
  n: number;
  p: string;
  title: string;
  state: CardState;
  role: string;
  agent: string;
  model: string;
  think: string | null;
  perm: string;
  branch: string | null;
  ci: CIState | null;
  doing: string;
  reason: string;
  pinned: boolean;
  paused: boolean;
  /** The stored state of the card's session, or null when it never had one (section S7c). */
  session: SessionState | null;
  /** The session is asleep or waking: see `sleepFlags`. */
  asleep: boolean;
  /** The session is being resumed. */
  waking: boolean;
  /** Which view the card opens in (section S9): the chat view, or the agent's own terminal. */
  viewMode: CardViewMode;
  pkg: string | null;
  labels: string[];
  s: number | null;
  e: number | null;
  due: number | null;
  ctx: number;
  upd: number;
  pr: number | null;
}

/** A card as the daemon knows it, in the words the screens use for the same things. */
export function toStoredCard(card: WireCard, now: number): DaemonCard {
  return {
    id: card.key,
    daemonId: card.id,
    n: card.number,
    p: card.projectId,
    title: card.title,
    state: card.state,
    role: card.role,
    agent: AGENT_NAMES[card.agent] ?? card.agent,
    model: card.model,
    think: card.thinking ? (THINK_NAMES[card.thinking] ?? null) : null,
    perm: PERM_NAMES[card.permissionMode] ?? card.permissionMode,
    branch: card.branch === "" ? null : card.branch,
    ci: card.ci ?? null,
    doing: card.doingNow,
    reason: card.needsReason?.text ?? "",
    pinned: card.pinned,
    paused: card.paused,
    session: card.session ?? null,
    ...sleepFlags(card.session),
    viewMode: card.viewMode,
    pkg: card.package === "" ? null : card.package,
    labels: card.labels.map((label) => label.name),
    s: dayOf(now, card.plannedStart ? toMillis(card.plannedStart) : null),
    e: dayOf(now, card.plannedEnd ? toMillis(card.plannedEnd) : null),
    due: dayOf(now, card.due ? toMillis(card.due) : null),
    ctx: card.contextUsed / PERCENT,
    upd: toMillis(card.updatedAt),
    pr: card.pullRequest?.number ?? null,
  };
}

/**
 * The fields the daemon fills, so a card that is already in the store is updated in place and only
 * what changed redraws. What names a card (id, p, n) never changes, and the mock-only fields are
 * not here at all, so whatever the app keeps in them is left alone.
 */
export type MirroredCard = Omit<DaemonCard, "id" | "p" | "n">;

/** The daemon's values for the fields it owns on a card the store already has. */
export function mirroredCard(card: WireCard, now: number): MirroredCard {
  const { id: _key, p: _project, n: _number, ...mirrored } = toStoredCard(card, now);
  return mirrored;
}

/** The agent kind a screen's agent name means, or undefined when the catalog does not know it. */
export function agentKindOf(name: string): AgentKind | undefined {
  return AGENT_KINDS[name];
}

/** The permission mode a screen's name means, or undefined when it is not one Marshal knows. */
export function permissionModeOf(name: string): PermissionMode | undefined {
  return PERM_MODES[name];
}

/** The thinking mode a screen's name means, or undefined when it is not one Marshal knows. */
export function thinkingModeOf(name: string): ThinkingMode | undefined {
  return THINK_MODES[name];
}
