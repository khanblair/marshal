import { createRoot, createSignal } from "solid-js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type ChatScroll, useChatScroll } from "./use-chat-scroll";

const SCROLL_HEIGHT_PX = 1000;
const VIEW_HEIGHT_PX = 400;
const AT_BOTTOM_PX = SCROLL_HEIGHT_PX - VIEW_HEIGHT_PX;
const MOUNT_DELAY_MS = 50;

interface Harness {
  scroll: ChatScroll;
  el: HTMLDivElement;
  setSignature: (value: string) => void;
  scrollTo: (top: number) => void;
  dispose: () => void;
}

function mount(): Harness {
  const el = { scrollTop: 0, scrollHeight: SCROLL_HEIGHT_PX, clientHeight: VIEW_HEIGHT_PX };
  const [signature, setSignature] = createSignal("ch1:1:5");
  let scroll: ChatScroll | undefined;
  const dispose = createRoot((dispose) => {
    scroll = useChatScroll(signature);
    scroll.setEl(el as HTMLDivElement);
    return dispose;
  });
  if (!scroll) throw new Error("hook did not run");
  const api = scroll;
  return {
    scroll: api,
    el: el as HTMLDivElement,
    setSignature,
    scrollTo: (top) => {
      el.scrollTop = top;
      api.onScroll({ currentTarget: el } as unknown as Event & { currentTarget: HTMLElement });
    },
    dispose,
  };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("useChatScroll", () => {
  it("scrolls to the newest message on mount", () => {
    const h = mount();
    vi.advanceTimersByTime(MOUNT_DELAY_MS);
    expect(h.el.scrollTop).toBe(SCROLL_HEIGHT_PX);
    expect(h.scroll.showJump()).toBe(false);
    h.dispose();
  });

  it("follows a new message while the reader is at the bottom", () => {
    const h = mount();
    vi.advanceTimersByTime(MOUNT_DELAY_MS);
    h.scrollTo(AT_BOTTOM_PX);
    h.el.scrollTop = 10;
    h.setSignature("ch1:2:9");
    vi.advanceTimersByTime(0);
    expect(h.el.scrollTop).toBe(SCROLL_HEIGHT_PX);
    expect(h.scroll.showJump()).toBe(false);
    h.dispose();
  });

  it("shows Jump to latest when a message arrives while the reader is scrolled up", () => {
    const h = mount();
    vi.advanceTimersByTime(MOUNT_DELAY_MS);
    h.scrollTo(0);
    expect(h.scroll.showJump()).toBe(false);
    h.setSignature("ch1:2:9");
    vi.advanceTimersByTime(MOUNT_DELAY_MS);
    expect(h.el.scrollTop).toBe(0);
    expect(h.scroll.showJump()).toBe(true);
    h.scroll.jump();
    expect(h.scroll.showJump()).toBe(false);
    expect(h.el.scrollTop).toBe(SCROLL_HEIGHT_PX);
    h.dispose();
  });

  it("hides Jump to latest when the reader scrolls back down", () => {
    const h = mount();
    vi.advanceTimersByTime(MOUNT_DELAY_MS);
    h.scrollTo(0);
    h.setSignature("ch1:2:9");
    expect(h.scroll.showJump()).toBe(true);
    h.scrollTo(AT_BOTTOM_PX);
    expect(h.scroll.showJump()).toBe(false);
    h.dispose();
  });

  it("scrolls instead of showing Jump when another chat opens", () => {
    const h = mount();
    vi.advanceTimersByTime(MOUNT_DELAY_MS);
    h.scrollTo(0);
    h.setSignature("ch2:3:9");
    vi.advanceTimersByTime(0);
    expect(h.el.scrollTop).toBe(SCROLL_HEIGHT_PX);
    expect(h.scroll.showJump()).toBe(false);
    h.dispose();
  });

  it("goes back to the bottom after a send and when a chat opens from the list", () => {
    const h = mount();
    vi.advanceTimersByTime(MOUNT_DELAY_MS);
    h.scrollTo(0);
    h.setSignature("ch1:2:9");
    expect(h.scroll.showJump()).toBe(true);
    h.scroll.afterSend();
    vi.advanceTimersByTime(0);
    expect(h.scroll.showJump()).toBe(false);
    expect(h.el.scrollTop).toBe(SCROLL_HEIGHT_PX);
    h.scrollTo(0);
    h.setSignature("ch1:3:9");
    h.scroll.leaveAway();
    expect(h.scroll.showJump()).toBe(false);
    h.dispose();
  });

  it("stops its timers when the view goes away", () => {
    const h = mount();
    h.dispose();
    vi.advanceTimersByTime(MOUNT_DELAY_MS);
    expect(h.el.scrollTop).toBe(0);
  });
});
