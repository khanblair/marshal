import { createEffect, createSignal, onCleanup, onMount, untrack } from "solid-js";
import { AWAY_THRESHOLD_PX, signatureChatId } from "./chat-model";

/** The design waits this long after mount so the thread has laid out before the first scroll. */
const MOUNT_SCROLL_DELAY_MS = 50;

export interface ChatScroll {
  /** Gets the scrolling element. Call it from a `ref`. */
  setEl: (el: HTMLDivElement) => void;
  onScroll: (event: Event & { currentTarget: HTMLElement }) => void;
  /** True when the user has scrolled away from the newest message and something new arrived. */
  showJump: () => boolean;
  /** A message was sent: back at the newest message, with nothing unseen. */
  afterSend: () => void;
  /** A chat was opened from the list: the reader is no longer away. Unseen is left as it is. */
  leaveAway: () => void;
  /** Jump to latest: scrolls to the newest message now. */
  jump: () => void;
}

/**
 * Keeps a message list pinned to the newest message, like the design: a change of chat, or a new
 * message while the reader is at the bottom, scrolls down; a new message while the reader has
 * scrolled up only marks it unseen, which shows Jump to latest.
 */
export function useChatScroll(signature: () => string): ChatScroll {
  const [away, setAway] = createSignal(false);
  const [unseen, setUnseen] = createSignal(false);
  let el: HTMLDivElement | undefined;
  let previous: string | undefined;
  const timers = new Set<ReturnType<typeof setTimeout>>();

  const toBottom = () => {
    if (el) el.scrollTop = el.scrollHeight;
  };
  const later = (ms: number) => {
    const id = setTimeout(() => {
      timers.delete(id);
      toBottom();
    }, ms);
    timers.add(id);
  };
  onMount(() => later(MOUNT_SCROLL_DELAY_MS));
  onCleanup(() => {
    for (const id of timers) clearTimeout(id);
  });

  createEffect(() => {
    const next = signature();
    if (next === previous) return;
    const changedChat =
      previous === undefined || signatureChatId(previous) !== signatureChatId(next);
    previous = next;
    if (changedChat || !untrack(away)) later(0);
    else if (!untrack(unseen)) setUnseen(true);
  });

  const reset = () => {
    setAway(false);
    setUnseen(false);
  };
  return {
    setEl: (node) => {
      el = node;
    },
    onScroll: (event) => {
      const box = event.currentTarget;
      const nowAway = box.scrollHeight - box.scrollTop - box.clientHeight > AWAY_THRESHOLD_PX;
      if (nowAway === untrack(away)) return;
      setAway(nowAway);
      if (!nowAway) setUnseen(false);
    },
    showJump: () => away() && unseen(),
    afterSend: () => {
      reset();
      later(0);
    },
    leaveAway: () => setAway(false),
    jump: () => {
      reset();
      toBottom();
    },
  };
}
