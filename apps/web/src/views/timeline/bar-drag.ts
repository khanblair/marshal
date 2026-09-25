import { batch, createSignal, onCleanup } from "solid-js";
import { type Card, M } from "~/mock";
import { trackPointer } from "./pointer-track";
import { type BarDrag, DRAG_MOVE_PX, snapDays } from "./timeline-geometry";
import { breakMessage, planMove } from "./timeline-model";

/** Moves the card's dates, asking first when the move breaks a dependency. */
function drop(c: Card, delta: number): void {
  const plan = planMove(c, delta, M.S.cards, (id) => M.card(id));
  const apply = (): void =>
    batch(() => {
      c.s = plan.s;
      c.e = plan.e;
      M.toast("Dates updated");
    });
  if (!plan.broken.length) {
    apply();
    return;
  }
  M.confirm({
    title: "Break a dependency",
    message: breakMessage(plan.broken),
    action: "Move anyway",
    run: apply,
  });
}

/**
 * Dragging timeline bars: the bar follows the pointer in whole days and the drop moves
 * the card's planned dates. A press without movement opens the card instead.
 */
export function createBarDrag() {
  const [drag, setDrag] = createSignal<BarDrag | null>(null);
  let stop: (() => void) | undefined;
  const release = (): void => {
    stop?.();
    stop = undefined;
  };
  onCleanup(release);

  const down = (e: PointerEvent, c: Card): void => {
    if (e.button !== 0) return;
    e.preventDefault();
    release();
    let moved = false;
    stop = trackPointer(e.clientX, {
      move: (dx) => {
        const delta = snapDays(dx);
        if (Math.abs(dx) > DRAG_MOVE_PX) moved = true;
        const current = drag();
        if (!current || current.delta !== delta) setDrag({ id: c.id, delta });
      },
      up: () => {
        stop = undefined;
        const last = drag();
        setDrag(null);
        if (!moved) M.openCard(c.id);
        else if (last?.delta) drop(c, last.delta);
      },
      cancel: () => {
        stop = undefined;
        setDrag(null);
      },
    });
  };

  return { drag, down };
}
