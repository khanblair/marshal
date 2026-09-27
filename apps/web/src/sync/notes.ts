import { toNoteInfo } from "~/data/mappers/notes";
import { ChangeInFlightError } from "~/data/optimistic";
import type { CardKey } from "~/mock/card-key";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";

/*
 * The one write the Notes tab makes once section S14 is the daemon's: replacing a card's whole
 * note. Reading a note is `sync/card-session.ts`'s `readOpenCard`, run once when the card opens,
 * because a note has no live event of its own to keep it fresh after that (`docs/architecture.md`
 * section 12: the vault watcher is what catches an edit made in Obsidian, on its own 30s sweep, not
 * a card's event stream).
 */

const NOT_CONNECTED = "Marshal is not connected to its daemon.";

/**
 * Saves a card's note through the daemon and keeps the store's own two fields - the body
 * (`M.S.notes`, unchanged in shape since the mock) and the daemon's metadata (`M.S.noteInfo`) - in
 * step with what it answered. False means nothing was asked, or the daemon refused it; either way
 * the reason has already been shown as a toast, the same rule every write in this module follows.
 */
export async function saveCardNote(ctx: Ctx, key: CardKey, body: string): Promise<boolean> {
  const card = ctx.S.cards.find((one) => one.id === key);
  const daemonId = card?.daemonId ?? "";
  if (!card || !daemonId) return false;
  const api = ctx.env.data?.api;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return false;
  }
  try {
    await ctx.optimistic({
      key: `note:${key}`,
      // The editor already shows what the person typed; there is nothing else to draw ahead of the
      // daemon's own answer.
      apply: () => undefined,
      request: async () => {
        const note = await api.saveNote(daemonId, { body });
        ctx.S.notes = { ...ctx.S.notes, [key]: note.body };
        ctx.S.noteInfo = { ...ctx.S.noteInfo, [key]: toNoteInfo(note) };
        toast(ctx, "Note saved");
      },
      rollback: () => undefined,
    });
    return true;
  } catch (error) {
    if (error instanceof ChangeInFlightError) toast(ctx, error.message);
    return false;
  }
}
