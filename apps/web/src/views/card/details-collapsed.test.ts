import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const LEGACY_KEY = "marshal.card.details.collapsed";
const DESKTOP_KEY = "marshal.card.details.collapsed.desktop";
const MOBILE_KEY = "marshal.card.details.collapsed.mobile";

beforeEach(() => {
  window.localStorage.clear();
  vi.resetModules();
});
afterEach(() => window.localStorage.clear());

describe("card detail preferences", () => {
  it("starts mobile folded and reads the old preference for desktop only", async () => {
    window.localStorage.setItem(LEGACY_KEY, "1");
    const details = await import("./details-collapsed");
    expect(details.detailsCollapsed(true)).toBe(true);
    expect(details.detailsCollapsed()).toBe(true);

    window.localStorage.clear();
    vi.resetModules();
    const fresh = await import("./details-collapsed");
    expect(fresh.detailsCollapsed(true)).toBe(true);
    expect(fresh.detailsCollapsed()).toBe(false);
  });

  it("stores explicit independent show and hide choices for mobile and desktop", async () => {
    const details = await import("./details-collapsed");
    expect(details.detailsCollapsed(true)).toBe(true);
    expect(details.detailsCollapsed()).toBe(false);

    details.toggleDetails(true);
    details.toggleDetails();
    expect(window.localStorage.getItem(MOBILE_KEY)).toBe("0");
    expect(window.localStorage.getItem(DESKTOP_KEY)).toBe("1");

    vi.resetModules();
    const reloaded = await import("./details-collapsed");
    expect(reloaded.detailsCollapsed(true)).toBe(false);
    expect(reloaded.detailsCollapsed()).toBe(true);
  });
});
