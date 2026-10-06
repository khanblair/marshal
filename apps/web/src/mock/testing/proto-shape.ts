/**
 * The port and the design prototype agree on everything except how a card is named. The
 * prototype numbers cards across all projects (`41`, and `300` for the first new card); the port
 * names a card by its project and number (`api#41`, and `api#47` for the first new card of the
 * api project) and shows the project name beside the number in lists that can mix projects.
 * This module translates between the two so the differential tests can keep comparing the
 * whole state: call arguments go to the prototype in its numbers, and the port's values are
 * read back in the prototype's shape. A project from the daemon also has three fields the
 * prototype's projects lack, and the state has some of its own the prototype's lacks (the daemon's
 * catalog, the stored Home numbers, a card's restore points, and the keep-awake length), and they
 * are left out. Nothing here hides any other difference.
 */
import { cardNumber, parseCardKey } from "../card-key";
import type { Marshal } from "../marshal";
import type { ProtoM } from "./prototype";

/** The per-card maps of the store, keyed by card. */
const CARD_MAPS = ["chat", "act", "checks", "notes", "preview"] as const;
/**
 * What the daemon knows about a project that the prototype's seed projects do not have: the
 * default and integration branches, the dev command, and the bypass lock. A project in the port
 * always has them.
 */
const DAEMON_PROJECT_FIELDS = ["branch", "integrationBranch", "dev", "lockBypass"] as const;
/** A schedule's own project id, which round-trips a save to the daemon; the prototype has no such
 * field, since it never saves a schedule anywhere. */
/** Connections the port lists that the prototype never drew, so its seed has no row for them. */
const PORT_ONLY_INTEGRATIONS = ["ntfy", "gdrive", "gdocs", "gsheets", "gslides"];
const DAEMON_SCHEDULE_FIELDS = ["projectId"] as const;
/** Fake pull request number of a card moved to review by hand: this plus the card number. */
const FAKE_PR_BASE = 300;

type Json = Record<string, unknown>;

export interface ProtoShape {
  /** The prototype's id of the card with this key, found by its place in `S.cards`. */
  protoId(key: string): number | undefined;
  /** A call argument in the prototype's terms: card keys become its numeric ids. */
  arg(value: unknown): unknown;
  /** A snapshot of the port in the prototype's shape. */
  shape(value: unknown): unknown;
}

const isObject = (value: unknown): value is Json =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const isProject = (value: Json): boolean =>
  ["id", "name", "lang", "path"].every((key) => typeof value[key] === "string");

const isSchedule = (value: Json): boolean =>
  typeof value.trigger === "string" &&
  typeof value.missed === "string" &&
  Array.isArray(value.days);

function projectFields(project: Json): Json {
  const rest = { ...project };
  for (const key of DAEMON_PROJECT_FIELDS) delete rest[key];
  return rest;
}

function scheduleFields(schedule: Json): Json {
  const rest = { ...schedule };
  for (const key of DAEMON_SCHEDULE_FIELDS) delete rest[key];
  return rest;
}

/**
 * The whole state without what the port keeps beyond the prototype's fields:
 *
 * - `agents`, the daemon's catalog (the prototype's fixed table is in `M.AGENTS`), `agentTools`, the
 *   other agent programs it found, `stats`, the
 *   stored Home numbers (`sync/home-stats.ts`), and `calGoogle`, how reading Google Calendar went
 *   (`sync/calendar.ts`).
 * - `checkpoints`, a card's restore points, which the port keeps in the store (Phase 5, B5.3) and
 *   fills from the daemon's own activity. The prototype keeps no such state: its `CardDetail.dc.html`
 *   makes three fake rows inside `renderVals`, so there is nothing here to compare them against.
 * - A sleep settings object's `keepAwake`, the length "Keep awake" holds a card. It has no field on
 *   the screen, and the prototype writes the number straight into its sentence
 *   (`design/store.js`, "Kept awake for 15 more minutes").
 *
 * Anything else is left as it is.
 */
function withoutPortOnly(value: unknown): unknown {
  if (!isObject(value) || !Array.isArray(value.cards) || !("agents" in value)) return value;
  const {
    agents: _daemon,
    agentTools: _tools,
    stats: _numbers,
    calGoogle: _google,
    checkpoints: _restorePoints,
    ...rest
  } = value;
  const sleep = rest.sleep;
  const shown = isObject(sleep) ? { ...rest, sleep: sleepFields(sleep) } : rest;
  return Array.isArray(shown.integrations)
    ? {
        ...shown,
        integrations: shown.integrations.filter(
          (row) => !PORT_ONLY_INTEGRATIONS.includes(isObject(row) ? String(row.id) : ""),
        ),
      }
    : shown;
}

/** A sleep settings object without the port-only `keepAwake` length. */
function sleepFields(sleep: Json): Json {
  const { keepAwake: _kept, ...rest } = sleep;
  return rest;
}

const escapeRegExp = (text: string): string => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

interface CardPairs {
  /** The prototype's id of each port card, by key. */
  ids: Map<string, number>;
  /** The number of each port card whose number differs from the prototype's id, and that id. */
  renumbered: Map<number, number>;
}

/**
 * The prototype and the port add cards in the same order, so the card at a position in one is
 * the card at that position in the other.
 */
function pairCards(port: Pick<Marshal, "S">, proto: ProtoM): CardPairs {
  const protoCards = (proto.S as { cards: { id: number }[] }).cards;
  const ids = new Map<string, number>();
  const renumbered = new Map<number, number>();
  port.S.cards.forEach((card, i) => {
    const twin = protoCards[i];
    if (!twin) return;
    ids.set(card.id, twin.id);
    if (card.n !== twin.id) renumbered.set(card.n, twin.id);
  });
  return { ids, renumbered };
}

export function createProtoShape(port: Pick<Marshal, "S">, proto: ProtoM): ProtoShape {
  const { ids, renumbered } = pairCards(port, proto);
  const names = port.S.projects.map((p) => escapeRegExp(p.name));
  const namePrefix = names.length ? new RegExp(`^(${names.join("|")}) (?=#\\d)`) : null;
  const nameInText = names.length ? new RegExp(`(?:${names.join("|")}) (?=#\\d)`, "g") : null;

  /** A card key as the prototype's id, also for a card that no longer exists. */
  const idOf = (key: string): number | undefined =>
    ids.get(key) ?? (parseCardKey(key) ? cardNumber(key) : undefined);

  const text = (value: string): string => {
    let out = nameInText ? value.replace(nameInText, "") : value;
    if (renumbered.size) {
      out = out.replace(/(#|marshal\/)(\d+)\b/g, (whole, lead: string, digits: string) => {
        const twin = renumbered.get(Number(digits));
        return twin === undefined ? whole : `${lead}${twin}`;
      });
    }
    return out;
  };

  function cardFields(card: Json): Json {
    const { n, ...rest } = card;
    const twin = typeof n === "number" ? renumbered.get(n) : undefined;
    if (twin !== undefined && rest.pr === FAKE_PR_BASE + (n as number)) {
      rest.pr = FAKE_PR_BASE + twin;
    }
    return rest;
  }

  const isCard = (value: Json): boolean =>
    typeof value.n === "number" && Array.isArray(value.deps) && "p" in value;

  /** The `hint` a Cards row gets in the prototype: the project name the port puts in the label. */
  function projectHint(source: Json): string | undefined {
    if (source.group !== "Cards" || typeof source.label !== "string") return undefined;
    return namePrefix?.exec(source.label)?.[1];
  }

  function shapeObject(value: Json): Json {
    let source = value;
    if (isCard(value)) source = cardFields(value);
    else if (isProject(value)) source = projectFields(value);
    else if (isSchedule(value)) source = scheduleFields(value);
    const out: Json = {};
    for (const [key, item] of Object.entries(source)) {
      if (key === "card" && typeof item === "string" && idOf(item) !== undefined) continue;
      out[String(idOf(key) ?? key)] = shape(item, key);
    }
    const hint = projectHint(source);
    if (hint) out.hint = hint;
    return out;
  }

  function shape(value: unknown, prop = ""): unknown {
    if (typeof value === "string") {
      // A card view's `key` is the port's card key (`api#41`); the prototype's was `c<number>`.
      // Nothing else reads `CardView.key`.
      if (prop === "key" && parseCardKey(value)) return `c${cardNumber(value)}`;
      if (prop === "target" && parseCardKey(value)) return `#${idOf(value)}`;
      return idOf(value) ?? text(value);
    }
    if (Array.isArray(value)) return value.map((item) => shape(item, prop));
    return isObject(value) ? shapeObject(value) : value;
  }

  function arg(value: unknown): unknown {
    if (typeof value === "string") return idOf(value) ?? value;
    if (Array.isArray(value)) return value.map(arg);
    if (!isObject(value)) return value;
    return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, arg(item)]));
  }

  return { protoId: (key) => idOf(key), arg, shape: (value) => shape(withoutPortOnly(value)) };
}

/**
 * Removes what a store keeps that the other side does not. The port forgets what it holds for a
 * card that is gone (a new card can take the number again, and the prototype never numbers
 * twice, so it keeps it), and it never shows a feed item of a project that is gone, where the
 * prototype keeps it. Both sides go through this before they are compared.
 */
export function withoutOrphans(state: unknown): unknown {
  if (!isObject(state) || !Array.isArray(state.cards)) return state;
  const live = new Set((state.cards as Json[]).map((card) => String(card.id)));
  const out: Json = { ...state };
  for (const name of CARD_MAPS) {
    const map = out[name];
    if (!isObject(map)) continue;
    out[name] = Object.fromEntries(Object.entries(map).filter(([id]) => live.has(id)));
  }
  if (Array.isArray(out.feed) && Array.isArray(out.projects)) {
    const projects = new Set((out.projects as Json[]).map((project) => String(project.id)));
    out.feed = (out.feed as Json[]).filter(
      (item) => item.pid == null || projects.has(String(item.pid)),
    );
  }
  return out;
}
