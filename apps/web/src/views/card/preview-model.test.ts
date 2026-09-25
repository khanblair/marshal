import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { previewState, previewStatus, previewUrl, togglePreview } from "./preview-model";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

describe("preview", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    resetStore();
  });
  afterEach(() => vi.useRealTimers());

  it("gives web cards a port from their id, and card 118 the settings page", () => {
    expect(previewUrl(cardOf(119))).toBe("http://localhost:5129/reports");
    expect(previewUrl(cardOf(118))).toBe("http://localhost:5128/settings");
  });

  it("gives mobile cards a login page on their own port range", () => {
    expect(previewUrl(cardOf(209))).toBe("http://localhost:8029/login");
  });

  it("describes each state", () => {
    expect(previewStatus(cardOf(119), "stopped")).toBe(
      "Stopped. Start the preview to run the app from this card's worktree.",
    );
    expect(previewStatus(cardOf(119), "starting")).toBe("Starting the dev server with pnpm dev");
    expect(previewStatus(cardOf(209), "starting")).toBe(
      "Starting the dev server with pnpm --filter apps/ios web",
    );
    expect(previewStatus(cardOf(119), "running")).toBe(
      "Running from marshal/119-tanstack-table on port 5129",
    );
    expect(previewStatus(cardOf(45), "running")).toBe("Running from main on port 5145");
  });

  it("starts, becomes running after the fake server boots, and stops with a toast", () => {
    expect(previewState(119)).toBe("stopped");
    togglePreview(119);
    expect(previewState(119)).toBe("starting");
    vi.advanceTimersByTime(1800);
    expect(previewState(119)).toBe("running");
    togglePreview(119);
    expect(previewState(119)).toBe("stopped");
    expect(M.S.toasts.at(-1)?.msg).toBe("Preview stopped");
  });
});
