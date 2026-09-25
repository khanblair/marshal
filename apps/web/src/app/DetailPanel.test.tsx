import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const cardStub = vi.hoisted(() => () => {
  const el = document.createElement("div");
  el.dataset.view = "card";
  return el;
});
vi.mock("~/views/card/CardDetail", () => ({ CardDetail: cardStub }));

const { DetailPanel } = await import("./DetailPanel");

const DESKTOP_PX = 1440;
const TABLET_PX = 820;
const PHONE_PX = 390;
const DEFAULT_WIDTH_PX = 600;

function openCard(width = DESKTOP_PX): void {
  M.setViewport(width, 900);
  M.set({ openId: null, detailExpanded: false, detailW: DEFAULT_WIDTH_PX, menu: null });
  M.go("project", "api", "board");
  M.openCard(41);
}

const handle = (): HTMLElement => screen.getByRole("separator", { name: "Resize card panel" });
const pointer = (type: string, clientX: number): MouseEvent =>
  new MouseEvent(type, { clientX, bubbles: true, cancelable: true });

beforeEach(() => {
  M._scale = 1;
  openCard();
});
afterEach(() => {
  cleanup();
  M._scale = 1;
});

describe("DetailPanel layout", () => {
  it("renders nothing while no card is open", () => {
    M.closeCard();
    render(() => <DetailPanel />);
    expect(screen.queryByRole("complementary")).toBeNull();
  });

  it("is a fixed-width side panel with a handle on desktop", () => {
    render(() => <DetailPanel />);
    const aside = screen.getByRole("complementary", { name: "Card detail" });
    expect(aside).toHaveClass("relative", "flex-none", "max-w-[60%]");
    expect(aside.style.width).toBe("600px");
  });

  it("keeps the width inside 480 and 760", () => {
    M.set({ detailW: 2000 });
    render(() => <DetailPanel />);
    expect(screen.getByRole("complementary").style.width).toBe("760px");
    M.set({ detailW: 100 });
    expect(screen.getByRole("complementary").style.width).toBe("480px");
  });

  it("fills the row and drops the handle when expanded", () => {
    M.set({ detailExpanded: true });
    render(() => <DetailPanel />);
    const aside = screen.getByRole("complementary");
    expect(aside).toHaveClass("relative", "flex-1", "min-w-0");
    expect(aside.style.width).toBe("");
    expect(screen.queryByRole("separator")).toBeNull();
  });

  it("is an overlay with a backdrop on tablets", () => {
    openCard(TABLET_PX);
    const { container } = render(() => <DetailPanel />);
    expect(screen.getByRole("complementary")).toHaveClass(
      "absolute",
      "right-0",
      "z-detail",
      "shadow-e2",
    );
    expect(container.querySelector('[aria-hidden="true"]')).toHaveClass("z-[140]", "absolute");
    expect(screen.queryByRole("separator")).toBeNull();
  });

  it("is a full screen page on phones with no backdrop", () => {
    openCard(PHONE_PX);
    const { container } = render(() => <DetailPanel />);
    expect(screen.getByRole("complementary")).toHaveClass("fixed", "inset-0", "z-[250]");
    expect(container.querySelector('[aria-hidden="true"]')).toBeNull();
  });
});

describe("Resize handle", () => {
  it("exposes its range and current width", () => {
    render(() => <DetailPanel />);
    expect(handle()).toHaveAttribute("aria-orientation", "vertical");
    expect(handle()).toHaveAttribute("aria-valuemin", "480");
    expect(handle()).toHaveAttribute("aria-valuemax", "760");
    expect(handle()).toHaveAttribute("aria-valuenow", "600");
    expect(handle()).toHaveAttribute("tabindex", "0");
  });

  it("moves 24 px per arrow key: left widens, right narrows", () => {
    render(() => <DetailPanel />);
    fireEvent.keyDown(handle(), { key: "ArrowLeft" });
    expect(M.S.detailW).toBe(624);
    fireEvent.keyDown(handle(), { key: "ArrowRight" });
    fireEvent.keyDown(handle(), { key: "ArrowRight" });
    expect(M.S.detailW).toBe(576);
    expect(handle()).toHaveAttribute("aria-valuenow", "576");
  });

  it("stops at the limits and ignores other keys", () => {
    M.set({ detailW: 750 });
    render(() => <DetailPanel />);
    fireEvent.keyDown(handle(), { key: "ArrowLeft" });
    expect(M.S.detailW).toBe(760);
    fireEvent.keyDown(handle(), { key: "Enter" });
    expect(M.S.detailW).toBe(760);
  });

  it("follows a drag to the left by widening the panel", () => {
    render(() => <DetailPanel />);
    fireEvent(handle(), pointer("pointerdown", 500));
    window.dispatchEvent(pointer("pointermove", 420));
    expect(M.S.detailW).toBe(680);
    window.dispatchEvent(pointer("pointermove", 300));
    expect(M.S.detailW).toBe(760);
  });

  it("stops following the pointer after it is released", () => {
    render(() => <DetailPanel />);
    fireEvent(handle(), pointer("pointerdown", 500));
    window.dispatchEvent(pointer("pointermove", 480));
    window.dispatchEvent(pointer("pointerup", 480));
    window.dispatchEvent(pointer("pointermove", 400));
    expect(M.S.detailW).toBe(620);
  });

  it("converts screen pixels through the frame scale", () => {
    M._scale = 0.5;
    render(() => <DetailPanel />);
    fireEvent(handle(), pointer("pointerdown", 500));
    window.dispatchEvent(pointer("pointermove", 450));
    expect(M.S.detailW).toBe(700);
  });

  it("stops listening when the panel goes away mid drag", () => {
    const view = render(() => <DetailPanel />);
    fireEvent(handle(), pointer("pointerdown", 500));
    view.unmount();
    window.dispatchEvent(pointer("pointermove", 400));
    expect(M.S.detailW).toBe(DEFAULT_WIDTH_PX);
  });
});
