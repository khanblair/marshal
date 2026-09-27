import { restoreCheckpoint as restoreOnDaemon } from "~/sync/checkpoint-actions";
import type { CardKey } from "../card-key";
import type { Ctx } from "../context";
import { confirm, toast } from "../engine";

/*
 * The one write a person makes with a card's restore points (B5.3): putting the card back to one of
 * them. The card's activity, and so its restore points, are the daemon's once section S10 is
 * switched, and the commit behind a restore point is the daemon's own, so a restore is a worktree
 * reset there and nothing here stands in for it.
 *
 * The screen asks first either way, because a restore throws work away. The words say what a restore
 * actually does: the worktree and the branch go back to the commit, and what was made since is
 * discarded. (The prototype's own copy promised a backup ref; the daemon keeps none, and the report
 * records the difference rather than the copy promising something that does not happen.)
 */
export function restoreCheckpoint(
  ctx: Ctx,
  id: CardKey,
  checkpointId: string,
  label: string,
): void {
  const card = ctx.S.cards.find((one) => one.id === id);
  const name = label.trim() || "this restore point";
  confirm(ctx, {
    title: "Restore checkpoint",
    message: `This resets the worktree and the branch to "${name}". Any change made since then is discarded.`,
    action: "Restore checkpoint",
    run: () => {
      if (!card?.daemonId) return;
      void restoreOnDaemon(ctx, id, checkpointId).then((restored) => {
        if (restored) toast(ctx, "Checkpoint restored");
      });
    },
  });
}
