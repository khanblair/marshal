import { describe, expect, it, vi } from "vitest";
import { trackPointer } from "./pointer-track";

const fire = (type: string, clientX = 0) =>
  window.dispatchEvent(new PointerEvent(type, { clientX }));

function handlers() {
  return { move: vi.fn(), up: vi.fn(), cancel: vi.fn() };
}

describe("trackPointer", () => {
  it("reports the horizontal distance from the press on every move", () => {
    const on = handlers();
    const stop = trackPointer(100, on);
    fire("pointermove", 130);
    fire("pointermove", 60);
    expect(on.move.mock.calls).toEqual([[30], [-40]]);
    stop();
  });

  it("reports the release once and stops listening", () => {
    const on = handlers();
    trackPointer(0, on);
    fire("pointerup");
    fire("pointerup");
    fire("pointermove", 50);
    expect(on.up).toHaveBeenCalledOnce();
    expect(on.move).not.toHaveBeenCalled();
  });

  it("reports a cancel once and stops listening", () => {
    const on = handlers();
    trackPointer(0, on);
    fire("pointercancel");
    fire("pointerup");
    expect(on.cancel).toHaveBeenCalledOnce();
    expect(on.up).not.toHaveBeenCalled();
  });

  it("stops listening when the returned function is called", () => {
    const on = handlers();
    const stop = trackPointer(0, on);
    stop();
    fire("pointermove", 10);
    fire("pointerup");
    expect(on.move).not.toHaveBeenCalled();
    expect(on.up).not.toHaveBeenCalled();
  });
});
