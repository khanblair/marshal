import { createRoot } from "solid-js";
import { describe, expect, it } from "vitest";
import { createTerminalState } from "./terminal-state";

const withState = <T>(run: (state: ReturnType<typeof createTerminalState>) => T): T =>
  createRoot((dispose) => {
    const result = run(createTerminalState());
    dispose();
    return result;
  });

describe("terminal state", () => {
  it("toggles Ctrl and logs nothing for it", () => {
    withState((state) => {
      state.press("Ctrl", "Control key");
      expect(state.ctrl()).toBe(true);
      expect(state.keyLog()).toEqual([]);
      state.press("Ctrl", "Control key");
      expect(state.ctrl()).toBe(false);
    });
  });

  it("logs a key, with Ctrl+ in front when armed, and disarms Ctrl", () => {
    withState((state) => {
      state.press("Esc", "Escape key");
      state.press("Ctrl", "Control key");
      state.press("Tab", "Tab key");
      expect(state.keyLog()).toEqual(["Escape key", "Ctrl+Tab key"]);
      expect(state.ctrl()).toBe(false);
    });
  });
});
