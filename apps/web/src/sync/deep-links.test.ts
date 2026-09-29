import { createRoot } from "solid-js";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createPlatform } from "~/platform";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { applyDeepLink, followDeepLinks, resolveDeepLink } from "./deep-links";

afterEach(() => window.history.replaceState(null, "", "/"));

describe("resolveDeepLink", () => {
  it("reads a card link, an approval link, and the card link on the daemon's own page", () => {
    expect(resolveDeepLink("marshal://card/01ABC")).toEqual({ kind: "card", cardId: "01ABC" });
    expect(resolveDeepLink("marshal://approval/ap1?card=01ABC")).toEqual({
      kind: "card",
      cardId: "01ABC",
    });
    expect(resolveDeepLink("http://laptop.tail1.ts.net:47800/?open=card/01ABC")).toEqual({
      kind: "card",
      cardId: "01ABC",
    });
  });

  it("reads shared text, taking the title from the subject or else the first line", () => {
    expect(resolveDeepLink("marshal://share?text=Fix%20login%0Amore&title=Bug")).toEqual({
      kind: "share",
      title: "Bug",
      body: "Fix login\nmore",
    });
    expect(resolveDeepLink("marshal://share?text=%0A%20Fix%20login%0Amore")).toMatchObject({
      title: "Fix login",
    });
    const long = "x".repeat(200);
    expect(resolveDeepLink(`marshal://share?text=${long}`)).toMatchObject({
      title: "x".repeat(80),
    });
  });

  it("reads nothing that is not Marshal's own, or that has nothing in it", () => {
    for (const other of [
      "https://example.com/",
      "marshal://card/",
      "marshal://approval/ap1",
      "marshal://share",
      "marshal://other/x",
      "not a link",
      "http://a/?open=project/x",
    ]) {
      expect(resolveDeepLink(other), other).toBeNull();
    }
  });
});

describe("applyDeepLink", () => {
  it("opens the card the daemon's id names, and says so when it is not there", () => {
    const ctx = contextOf(createTestMarshal());
    const card = ctx.S.cards[0];
    if (!card) throw new Error("no seed card");
    card.daemonId = "01ABC";
    expect(applyDeepLink(ctx, { kind: "card", cardId: "01ABC" })).toBe(true);
    expect(ctx.S.openId).toBe(card.id);
    ctx.S.openId = null;
    expect(applyDeepLink(ctx, { kind: "card", cardId: "nope" })).toBe(false);
    expect(ctx.S.openId).toBeNull();
    expect(ctx.S.toasts.at(-1)?.msg).toBe("That card is not here. It may have been removed.");
  });

  it("opens the New card dialog with what was shared, in the project the person is on", () => {
    const ctx = contextOf(createTestMarshal());
    const pid = ctx.S.projects[0]?.id;
    expect(applyDeepLink(ctx, { kind: "share", title: "Bug", body: "Fix login" })).toBe(true);
    expect(ctx.S.newCard).toMatchObject({ title: "Bug", body: "Fix login" });
    expect(ctx.S.route).toMatchObject({ page: "project", pid });
  });
});

describe("followDeepLinks", () => {
  it("keeps a link until the app is ready, then shows it once", () => {
    const ctx = contextOf(createTestMarshal());
    const card = ctx.S.cards[0];
    if (!card) throw new Error("no seed card");
    card.daemonId = "01ABC";
    ctx.S.ready = false;
    ctx.S.openId = null;
    let hand: (url: string) => void = () => undefined;
    const device = {
      ...createPlatform("web"),
      onDeepLink: (handler: (url: string) => void) => {
        hand = handler;
        return () => undefined;
      },
    };
    const dispose = createRoot((stop) => {
      followDeepLinks(ctx, device);
      return stop;
    });
    hand("marshal://card/01ABC");
    expect(ctx.S.openId).toBeNull();
    ctx.S.ready = true;
    expect(ctx.S.openId).toBe(card.id);
    dispose();
  });

  it("follows the link the page was opened with and takes it out of the address", () => {
    const ctx = contextOf(createTestMarshal());
    const card = ctx.S.cards[0];
    if (!card) throw new Error("no seed card");
    card.daemonId = "01ABC";
    ctx.S.ready = true;
    ctx.S.openId = null;
    window.history.replaceState(null, "", "/?open=card/01ABC&x=1");
    const stop = vi.fn();
    const dispose = createRoot((off) => {
      followDeepLinks(ctx, { ...createPlatform("web"), onDeepLink: () => stop });
      return off;
    });
    expect(ctx.S.openId).toBe(card.id);
    expect(window.location.search).toBe("?x=1");
    dispose();
  });
});
