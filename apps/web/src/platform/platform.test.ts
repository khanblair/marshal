import { afterEach, describe, expect, it, vi } from "vitest";
import { createPlatform, detectKind } from ".";

const ANDROID = "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36";
const MAC = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)";

afterEach(() => {
  vi.restoreAllMocks();
  vi.doUnmock("@tauri-apps/plugin-haptics");
  vi.doUnmock("@tauri-apps/plugin-barcode-scanner");
  vi.doUnmock("@tauri-apps/plugin-deep-link");
  vi.resetModules();
});

describe("detectKind", () => {
  it("is the phone app only inside a native shell that reports a phone", () => {
    expect(detectKind({ tauri: false, userAgent: ANDROID })).toBe("web");
    expect(detectKind({ tauri: true, userAgent: ANDROID })).toBe("mobile");
    expect(detectKind({ tauri: true, userAgent: MAC })).toBe("desktop");
    expect(detectKind({ tauri: false, userAgent: MAC })).toBe("web");
  });
});

/** Lets the picker's own click run, then answers the input it made with the files a person chose. */
function answerChooser(files: File[] | null): HTMLInputElement[] {
  const inputs: HTMLInputElement[] = [];
  vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(function (
    this: HTMLInputElement,
  ) {
    inputs.push(this);
    queueMicrotask(() => {
      if (files === null) return this.dispatchEvent(new Event("cancel"));
      Object.defineProperty(this, "files", { value: files });
      this.dispatchEvent(new Event("change"));
    });
  });
  return inputs;
}

describe("the web platform", () => {
  const web = createPlatform("web");

  it("is a browser tab with no native abilities", async () => {
    expect(web).toMatchObject({ kind: "web", native: false, canScanCode: false });
    expect(await web.pickFolder()).toBeNull();
    expect(await web.scanCode()).toBeNull();
    expect(web.onDeepLink(() => undefined)).toBeTypeOf("function");
  });

  it("opens a chooser with the accepted types and answers what was picked", async () => {
    const png = new File(["x"], "a.png", { type: "image/png" });
    const inputs = answerChooser([png]);
    expect(await web.pickFiles({ accept: "image/png", multiple: true })).toEqual([png]);
    expect(inputs[0]).toMatchObject({ type: "file", accept: "image/png", multiple: true });
    expect(inputs[0]?.hasAttribute("capture")).toBe(false);
  });

  it("answers nothing when the chooser is closed, and asks for the camera when told to", async () => {
    const inputs = answerChooser(null);
    expect(await web.pickFiles({ camera: true })).toEqual([]);
    expect(inputs[0]?.getAttribute("capture")).toBe("environment");
  });

  it("vibrates briefly where the device can, and does nothing where it cannot", () => {
    const vibrate = vi.fn();
    Object.defineProperty(navigator, "vibrate", { value: vibrate, configurable: true });
    web.haptic("tap");
    expect(vibrate).toHaveBeenCalledWith(10);
    Reflect.deleteProperty(navigator, "vibrate");
    expect(() => web.haptic("error")).not.toThrow();
  });
});

describe("the desktop platform", () => {
  it("is native, has a folder chooser, and answers null when the dialog plugin is missing", async () => {
    const desktop = createPlatform("desktop");
    expect(desktop).toMatchObject({ kind: "desktop", native: true, canScanCode: false });
    expect(await desktop.pickFolder()).toBeNull();
  });
});

describe("the mobile platform", () => {
  it("scans a QR code through the camera plugin after asking for permission", async () => {
    const requestPermissions = vi.fn().mockResolvedValue("granted");
    const scan = vi.fn().mockResolvedValue({ content: "marshal://pair?code=7QX-2LD" });
    vi.doMock("@tauri-apps/plugin-barcode-scanner", () => ({
      Format: { QRCode: "QR_CODE" },
      checkPermissions: vi.fn().mockResolvedValue("prompt"),
      requestPermissions,
      scan,
    }));
    const mobile = createPlatform("mobile");
    expect(mobile).toMatchObject({ kind: "mobile", native: true, canScanCode: true });
    expect(await mobile.scanCode()).toBe("marshal://pair?code=7QX-2LD");
    expect(requestPermissions).toHaveBeenCalledOnce();
    expect(scan).toHaveBeenCalledWith({ formats: ["QR_CODE"] });
  });

  it("scans nothing when the camera is refused, or when the plugin fails", async () => {
    vi.doMock("@tauri-apps/plugin-barcode-scanner", () => ({
      Format: { QRCode: "QR_CODE" },
      checkPermissions: vi.fn().mockResolvedValue("denied"),
      requestPermissions: vi.fn().mockResolvedValue("denied"),
      scan: vi.fn(),
    }));
    expect(await createPlatform("mobile").scanCode()).toBeNull();
    vi.resetModules();
    vi.doMock("@tauri-apps/plugin-barcode-scanner", () => {
      throw new Error("no plugin");
    });
    expect(await createPlatform("mobile").scanCode()).toBeNull();
  });

  it("taps lightly, and marks success and error with the phone's own patterns", async () => {
    const impactFeedback = vi.fn().mockResolvedValue(null);
    const notificationFeedback = vi.fn().mockResolvedValue(null);
    vi.doMock("@tauri-apps/plugin-haptics", () => ({ impactFeedback, notificationFeedback }));
    const mobile = createPlatform("mobile");
    mobile.haptic("tap");
    await vi.waitFor(() => expect(impactFeedback).toHaveBeenCalledWith("light"));
    mobile.haptic("error");
    await vi.waitFor(() => expect(notificationFeedback).toHaveBeenCalledWith("error"));
  });

  it("hands over the link that opened the app and every later one, until stopped", async () => {
    let listener: (urls: string[]) => void = () => undefined;
    const off = vi.fn();
    vi.doMock("@tauri-apps/plugin-deep-link", () => ({
      getCurrent: vi.fn().mockResolvedValue(["marshal://card/api%2341"]),
      onOpenUrl: vi.fn(async (handler: (urls: string[]) => void) => {
        listener = handler;
        return off;
      }),
    }));
    const seen: string[] = [];
    const stop = createPlatform("mobile").onDeepLink((url) => seen.push(url));
    await vi.waitFor(() => expect(seen).toEqual(["marshal://card/api%2341"]));
    listener(["marshal://approval/abc"]);
    expect(seen).toEqual(["marshal://card/api%2341", "marshal://approval/abc"]);
    stop();
    expect(off).toHaveBeenCalledOnce();
  });
});
