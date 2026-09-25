import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createTestMarshal } from "~/testing/test-store";
import type { CardKey } from "../card-key";
import type { Marshal } from "../marshal";
import { applyTheme, watchSystemTheme } from "./theme";
import { watchResize } from "./viewport";

const make = (w = 1440): Marshal =>
  createTestMarshal({ hash: "#nosim", storage: null, viewport: { w, h: 900 }, applyTheme });

function pointer(type: string, x: number, y: number, init: PointerEventInit = {}): PointerEvent {
  return new PointerEvent(type, { clientX: x, clientY: y, button: 0, bubbles: true, ...init });
}

describe("card drag", () => {
  let M: Marshal;
  let cardEl: HTMLElement;
  let column: HTMLElement;
  beforeEach(() => {
    vi.useFakeTimers();
    M = make();
    column = document.createElement("div");
    column.setAttribute("data-col", "review");
    cardEl = document.createElement("div");
    cardEl.textContent = "#41";
    document.body.append(column, cardEl);
    document.elementFromPoint = () => column;
  });
  afterEach(() => {
    document.body.replaceChildren();
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  const press = (id: CardKey, init: PointerEventInit = {}): void => {
    cardEl.onpointerdown = (e) => {
      const c = M.card(id);
      if (c) M.deco(c).down(e);
    };
    cardEl.dispatchEvent(pointer("pointerdown", 10, 10, init));
  };

  it("drags a ghost copy and drops the card on a column", () => {
    press("api#41");
    window.dispatchEvent(pointer("pointermove", 12, 11));
    expect(document.body.children).toHaveLength(2);
    window.dispatchEvent(pointer("pointermove", 60, 40));
    const ghost = document.body.lastElementChild as HTMLElement;
    expect(ghost.style).toMatchObject({
      position: "fixed",
      zIndex: "450",
      transform: "translate(50px,30px)",
    });
    expect(M.S).toMatchObject({ dragId: "api#41", dropCol: "review" });
    expect(M.deco(M.card("api#41") as never).opacity).toBe("0.4");
    window.dispatchEvent(pointer("pointerup", 60, 40));
    expect(document.body.children).toHaveLength(2);
    expect(M.S).toMatchObject({ dragId: null, dropCol: null });
    expect(M.card("api#41")?.state).toBe("review");
    // The click that ends the drag must not open the card.
    M.deco(M.card("api#41") as never).open();
    expect(M.S.openId).toBeNull();
    vi.advanceTimersByTime(30);
    M.deco(M.card("api#41") as never).open();
    expect(M.S.openId).toBe("api#41");
  });

  it("treats a short press as a click, not a drag", () => {
    press("api#41");
    window.dispatchEvent(pointer("pointermove", 12, 12));
    window.dispatchEvent(pointer("pointerup", 12, 12));
    expect(M.S.dragId).toBeUndefined();
    expect(M.card("api#41")?.state).toBe("working");
  });

  it("does not drop on the card's own column or outside columns", () => {
    column.setAttribute("data-col", "working");
    press("api#41");
    window.dispatchEvent(pointer("pointermove", 80, 80));
    window.dispatchEvent(pointer("pointercancel", 80, 80));
    expect(M.card("api#41")?.state).toBe("working");
    document.elementFromPoint = () => null;
    press("api#41");
    window.dispatchEvent(pointer("pointermove", 80, 80));
    expect(M.S.dropCol).toBeNull();
    window.dispatchEvent(pointer("pointerup", 80, 80));
  });

  it("ignores other buttons, buttons inside the card, and phones", () => {
    press("api#41", { button: 2 });
    const inner = document.createElement("button");
    cardEl.append(inner);
    inner.dispatchEvent(pointer("pointerdown", 10, 10));
    M.setViewport(390, 844);
    press("api#41");
    window.dispatchEvent(pointer("pointermove", 80, 80));
    expect(M.S.dragId).toBeUndefined();
  });
});

describe("theme", () => {
  afterEach(() => {
    Reflect.deleteProperty(window, "matchMedia");
    document.documentElement.removeAttribute("data-theme");
  });

  it("applies light, dark, and the system setting", () => {
    const M = make();
    M.setTheme("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(M.S.resolvedTheme).toBe("dark");
    M.setTheme("system");
    expect(M.S.resolvedTheme).toBe("light");
  });

  it("follows the OS when it switches to dark", () => {
    let dark = false;
    let onChange = (): void => {};
    window.matchMedia = ((query: string) => ({
      get matches() {
        return dark && query.includes("dark");
      },
      addEventListener: (_: string, fn: () => void) => {
        onChange = fn;
      },
    })) as unknown as typeof window.matchMedia;
    const M = make();
    applyTheme(M.S);
    watchSystemTheme(M.S);
    expect(M.S.resolvedTheme).toBe("light");
    dark = true;
    onChange();
    expect(M.S.resolvedTheme).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });
});

describe("viewport", () => {
  it("follows the window width unless the app is framed", () => {
    const M = make(900);
    watchResize(M);
    window.dispatchEvent(new Event("resize"));
    expect(M.S.vw).toBe(window.innerWidth);
    M.set({ vw: 500 });
    M._framed = true;
    window.dispatchEvent(new Event("resize"));
    expect(M.S.vw).toBe(500);
  });
});
