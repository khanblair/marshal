import { cleanup, render } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { resetShell } from "./shell-test-utils";
import { useWindowViewport } from "./use-window-viewport";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

function Probe() {
  useWindowViewport();
  return null;
}

const resizeTo = (width: number, height: number) => {
  window.innerWidth = width;
  window.innerHeight = height;
  window.dispatchEvent(new Event("resize"));
};

const originalSize = { width: window.innerWidth, height: window.innerHeight };

beforeEach(() => resetShell());
afterEach(() => {
  cleanup();
  resizeTo(originalSize.width, originalSize.height);
});

describe("useWindowViewport", () => {
  it("sets the store's viewport to the window size and marks the store as framed", () => {
    render(() => <Probe />);
    expect(M.S.vw).toBe(window.innerWidth);
    expect(M.S.vh).toBe(window.innerHeight);
    expect(M._framed).toBe(true);
    expect(M._scale).toBe(1);
  });

  it("follows the window width and height together", () => {
    render(() => <Probe />);
    resizeTo(700, 500);
    expect(M.S.vw).toBe(700);
    expect(M.S.vh).toBe(500);
  });

  it("stops listening for resizes when it is removed", () => {
    const { unmount } = render(() => <Probe />);
    unmount();
    const before = M.S.vw;
    resizeTo(555, 400);
    expect(M.S.vw).toBe(before);
  });
});
