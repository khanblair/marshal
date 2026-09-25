import { createEffect, createMemo, on, onCleanup, onMount } from "solid-js";
import { M, type Msg } from "~/mock";
import type { Panel } from "./panel-state";

/** The user counts as scrolled away when this much of the chat is below the viewport. */
const AWAY_PX = 80;
/** Scroll to the newest output once the panel had time to lay out. */
const MOUNT_SCROLL_MS = 60;
/** The parts of the signature that say "the view changed", as opposed to "more output came in". */
const VIEW_PARTS = 3;

function lastSignature(list: readonly Msg[]): string {
  const last = list[list.length - 1];
  if (!last) return "";
  const text = "text" in last ? last.text : "";
  const state = "st" in last ? last.st : "";
  return `${text.length}${state}`;
}

const viewOf = (signature: string): string => signature.split(":").slice(0, VIEW_PARTS).join(":");

export interface AutoScroll {
  /** Ref of the chat scroller. */
  chatRef: (el: HTMLElement) => void;
  /** Ref of the terminal scroller. */
  termRef: (el: HTMLElement) => void;
  onScroll: (event: Event & { currentTarget: HTMLElement }) => void;
  /** The Jump to latest button. */
  jump: () => void;
}

/**
 * Keeps the newest output in view without jumping while you read: a new card, tab, or
 * mode scrolls to the bottom; new output scrolls only when you are near the bottom,
 * and otherwise shows Jump to latest.
 */
export function createAutoScroll(id: number, panel: Panel): AutoScroll {
  let chatEl: HTMLElement | undefined;
  let termEl: HTMLElement | undefined;
  const toBottom = () => {
    const el = chatEl ?? termEl;
    if (el) el.scrollTop = el.scrollHeight;
  };
  const signature = createMemo(() => {
    const list = M.S.chat[id] ?? [];
    return `${id}:${M.S.tab}:${M.S.mode}:${list.length}:${lastSignature(list)}`;
  });
  createEffect(
    on(signature, (next, previous) => {
      const first = previous === undefined || viewOf(previous) !== viewOf(next);
      if (first || !panel.state.away) setTimeout(toBottom, 0);
      else if (!panel.state.unseen) panel.set({ unseen: true });
    }),
  );
  onMount(() => setTimeout(toBottom, MOUNT_SCROLL_MS));
  return {
    chatRef: (el) => {
      chatEl = el;
      onCleanup(() => {
        if (chatEl === el) chatEl = undefined;
      });
    },
    termRef: (el) => {
      termEl = el;
      onCleanup(() => {
        if (termEl === el) termEl = undefined;
      });
    },
    onScroll: (event) => {
      const el = event.currentTarget;
      const away = el.scrollHeight - el.scrollTop - el.clientHeight > AWAY_PX;
      if (away === panel.state.away) return;
      panel.set({ away, unseen: away ? panel.state.unseen : false });
    },
    jump: () => {
      panel.set({ away: false, unseen: false });
      toBottom();
    },
  };
}
