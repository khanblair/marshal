/**
 * Id counters, shared the same way as in the prototype: `mid` numbers messages,
 * chats, activity, toasts, notices, feed items, and new projects; `ck` numbers
 * checklists, checklist items, and comments. Cards are not numbered here: each
 * project numbers its own cards (`nextCardNumber` in `card-key.ts`).
 * Seed data takes ids in a fixed order, so every seeded id matches the prototype.
 */
export interface IdCounters {
  mid: number;
  ck: number;
}

export const createIds = (): IdCounters => ({ mid: 1, ck: 1 });

export function takeMid(ids: IdCounters): number {
  const n = ids.mid;
  ids.mid = n + 1;
  return n;
}

export function takeCk(ids: IdCounters): number {
  const n = ids.ck;
  ids.ck = n + 1;
  return n;
}
