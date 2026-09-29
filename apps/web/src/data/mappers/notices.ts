import type { NoticeList, Notice as WireNotice } from "@marshal/protocol";
import { toMillis } from "./time";

/**
 * A group of idle cards that will sleep soon (architecture.md 5.1). The notices panel draws the
 * countdown from `deadline`, and the cards it names are the rows under it.
 *
 * The store's shapes live here in the data layer, not in `mock/`, so the mapper never depends on the
 * mock; `mock/types.ts` re-exports them for the store's own modules.
 */
export interface SleepNotice {
  id: string;
  kind: "sleep";
  /** The cards in the group, by their wire ids, which are the store's own card keys. */
  cards: string[];
  /** The moment the group sleeps, in the milliseconds the screens keep. */
  deadline: number;
  ts: number;
}

/** An informational notice: one line and a second line, with no cards behind it. */
export interface InfoNotice {
  id: string;
  kind: "ci-main" | "cost" | "plan" | "ci";
  /** The project it is about, when it is about one. */
  pid?: string;
  /** The card it is about. The wire has none of these yet, so it is absent today. */
  cardId?: string;
  text: string;
  sub: string;
  ts: number;
}

export type Notice = SleepNotice | InfoNotice;

/** The kinds that carry their own sentences, which is every kind but the sleep group. */
const INFO_KINDS: readonly string[] = ["ci-main", "cost", "plan", "ci"];

const isInfoKind = (kind: string): kind is InfoNotice["kind"] => INFO_KINDS.includes(kind);

/**
 * One wire notice as the store holds it, or null for a kind this build does not draw, and for a
 * sleep notice that names no card the store knows.
 *
 * Automatic sleep is the only notice Phase 5 sends, so the sleep group is the one that carries cards.
 * The daemon names them by its own opaque ids and every screen works in card keys, so `keyOf` (the
 * caller's lookup) does that translation; a card it cannot place is left out, and a group left with
 * no cards is not drawn at all, because there is no row to act on and the next list that does name
 * known cards brings the notice back.
 *
 * An informational notice travels with its own two sentences and the project it belongs to.
 *
 * A sleep notice's deadline is optional on the wire, and a notice without one cannot be drawn as a
 * countdown; the notice's own time stands in for it, so the row reads `0:00` - the same as a group
 * whose warning has run out - rather than "NaN".
 */
export function toNotice(
  wire: WireNotice,
  keyOf: (daemonId: string) => string | null,
): Notice | null {
  const ts = toMillis(wire.createdAt);
  if (wire.kind === "sleep") {
    const cards: string[] = [];
    for (const daemonId of wire.cards ?? []) {
      const key = keyOf(daemonId);
      if (key) cards.push(key);
    }
    if (cards.length === 0) return null;
    return {
      id: wire.id,
      kind: "sleep",
      cards,
      deadline: wire.deadline ? toMillis(wire.deadline) : ts,
      ts,
    };
  }
  if (!isInfoKind(wire.kind)) return null;
  return {
    id: wire.id,
    kind: wire.kind,
    pid: wire.projectId ?? undefined,
    text: wire.text ?? "",
    sub: wire.sub ?? "",
    ts,
  };
}

/** The daemon's whole list as the store holds it. A kind this build does not draw is left out. */
export function toNotices(list: NoticeList, keyOf: (daemonId: string) => string | null): Notice[] {
  const notices: Notice[] = [];
  for (const wire of list.notices) {
    const notice = toNotice(wire, keyOf);
    if (notice) notices.push(notice);
  }
  return notices;
}
