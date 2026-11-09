import { afterEach, describe, expect, it, vi } from "vitest";
import { createTestMarshal } from "~/testing/test-store";
import { applyTheme } from "~/mock/dom/theme";
import { setNativeSystemBars } from "./native-system-bars";

const originalUserAgent = Object.getOwnPropertyDescriptor(navigator, "userAgent");
const originalTauriInternals = Object.getOwnPropertyDescriptor(window, "__TAURI_INTERNALS__");

function restore(name: string, target: object, descriptor: PropertyDescriptor | undefined): void {
  if (descriptor) Object.defineProperty(target, name, descriptor);
  else Reflect.deleteProperty(target, name);
}

afterEach(() => {
  restore("userAgent", navigator, originalUserAgent);
  restore("__TAURI_INTERNALS__", window, originalTauriInternals);
  document.documentElement.removeAttribute("data-theme");
});

describe("native system bars", () => {
  it("sends the resolved app theme from applyTheme", () => {
    const invoke = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "userAgent", {
      configurable: true,
      value: "Mozilla/5.0 (Linux; Android 15)",
    });
    Object.defineProperty(window, "__TAURI_INTERNALS__", {
      configurable: true,
      value: { invoke },
    });
    const M = createTestMarshal();
    M.S.theme = "dark";

    applyTheme(M.S);

    expect(M.S.resolvedTheme).toBe("dark");
    expect(invoke).toHaveBeenCalledWith("plugin:marshal-mobile|set_system_bars", { dark: true });
  });

  it("sends only the resolved dark-mode boolean to Android Tauri", () => {
    const invoke = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "userAgent", {
      configurable: true,
      value: "Mozilla/5.0 (Linux; Android 15)",
    });
    Object.defineProperty(window, "__TAURI_INTERNALS__", {
      configurable: true,
      value: { invoke },
    });

    setNativeSystemBars(true);

    expect(invoke).toHaveBeenCalledOnce();
    expect(invoke).toHaveBeenCalledWith("plugin:marshal-mobile|set_system_bars", { dark: true });
  });

  it("does nothing outside Android Tauri and silently handles older APKs", async () => {
    const invoke = vi.fn().mockRejectedValue(new Error("command missing"));
    Object.defineProperty(navigator, "userAgent", {
      configurable: true,
      value: "Mozilla/5.0 (Macintosh; Intel Mac OS X)",
    });
    Object.defineProperty(window, "__TAURI_INTERNALS__", {
      configurable: true,
      value: { invoke },
    });
    setNativeSystemBars(false);
    expect(invoke).not.toHaveBeenCalled();

    Object.defineProperty(navigator, "userAgent", {
      configurable: true,
      value: "Mozilla/5.0 (Linux; Android 15)",
    });
    expect(() => setNativeSystemBars(false)).not.toThrow();
    await Promise.resolve();
    expect(invoke).toHaveBeenCalledWith("plugin:marshal-mobile|set_system_bars", { dark: false });
  });
});
