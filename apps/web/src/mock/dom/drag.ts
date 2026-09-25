import { batch } from "solid-js";
import { moveCard } from "../actions/cards";
import type { CardKey } from "../card-key";
import { colOf, isColumn } from "../constants";
import type { Ctx } from "../context";
import { set } from "../engine";
import { card, isMobile } from "../selectors";

/** The pointer must travel this far before a press becomes a drag. */
const DRAG_START_PX = 5;
const GHOST_Z_INDEX = "450";
/** The click that ends a drag arrives right after pointerup; ignore it for this long. */
const CLICK_GUARD_MS = 30;

interface DragSession {
  ctx: Ctx;
  id: CardKey;
  el: HTMLElement;
  startX: number;
  startY: number;
  rect: DOMRect;
  ghost: HTMLElement | null;
}

function startGhost(s: DragSession): HTMLElement | null {
  const ghost = s.el.cloneNode(true);
  if (!(ghost instanceof HTMLElement)) return null;
  Object.assign(ghost.style, {
    position: "fixed",
    left: `${s.rect.left}px`,
    top: `${s.rect.top}px`,
    width: `${s.rect.width}px`,
    zIndex: GHOST_Z_INDEX,
    pointerEvents: "none",
    boxShadow: "var(--elevation-drag)",
    margin: "0",
    opacity: "1",
    transition: "none",
  });
  document.body.appendChild(ghost);
  s.ctx.flags.suppressClick = true;
  set(s.ctx, { dragId: s.id });
  return ghost;
}

function onMove(s: DragSession, ev: PointerEvent): void {
  const dx = ev.clientX - s.startX;
  const dy = ev.clientY - s.startY;
  if (!s.ghost) {
    if (Math.hypot(dx, dy) < DRAG_START_PX) return;
    s.ghost = startGhost(s);
  }
  if (!s.ghost) return;
  s.ghost.style.transform = `translate(${dx}px,${dy}px)`;
  const under = document.elementFromPoint(ev.clientX, ev.clientY);
  const col = under?.closest("[data-col]")?.getAttribute("data-col") ?? null;
  if (col !== s.ctx.S.dropCol) set(s.ctx, { dropCol: col });
}

function onEnd(s: DragSession): void {
  if (!s.ghost) return;
  s.ghost.remove();
  const to = s.ctx.S.dropCol;
  set(s.ctx, { dragId: null, dropCol: null });
  const c = card(s.ctx, s.id);
  if (c && to && isColumn(to) && to !== colOf(c.state)) moveCard(s.ctx, s.id, to);
  setTimeout(() => {
    s.ctx.flags.suppressClick = false;
  }, CLICK_GUARD_MS);
}

/** Starts a pointer drag of a card: a ghost copy follows the pointer and the drop moves the card. */
export function dragStart(ctx: Ctx, e: PointerEvent, id: CardKey): void {
  if (e.button !== 0 || isMobile(ctx.S)) return;
  if (e.target instanceof Element && e.target.closest("button")) return;
  const el = e.currentTarget;
  if (!(el instanceof HTMLElement)) return;
  const s: DragSession = {
    ctx,
    id,
    el,
    startX: e.clientX,
    startY: e.clientY,
    rect: el.getBoundingClientRect(),
    ghost: null,
  };
  const move = (ev: PointerEvent): void => onMove(s, ev);
  const end = (): void => {
    window.removeEventListener("pointermove", move);
    window.removeEventListener("pointerup", end);
    window.removeEventListener("pointercancel", end);
    batch(() => onEnd(s));
  };
  window.addEventListener("pointermove", move);
  window.addEventListener("pointerup", end);
  window.addEventListener("pointercancel", end);
}
