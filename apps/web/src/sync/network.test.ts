import type { NetworkStatus, Event as WireEvent } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { sectionStatus } from "~/data/sections";
import { golden } from "~/data/testing/golden";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { applyNetwork, networkSyncer } from "./network";

const OFFLINE = golden<NetworkStatus>("network-status");
const ONLINE: NetworkStatus = { online: true, since: "2026-11-01T10:00:00.000Z" };

const changed = (data: NetworkStatus): WireEvent =>
  ({ topic: "home", type: "network.changed", data }) as unknown as WireEvent;

describe("the network section", () => {
  it("is section S33, on the daemon, and follows the home topic", () => {
    expect(networkSyncer.section).toBe("S33");
    expect(sectionStatus.S33).toBe("daemon");
    expect(networkSyncer.topics).toEqual(["home"]);
  });

  it("starts online, because the daemon checks only after a call failed", () => {
    const ctx = contextOf(createTestMarshal());
    expect(ctx.S.network).toEqual({ online: true, since: null });
  });

  it("mirrors what the daemon answered", () => {
    const ctx = contextOf(createTestMarshal());
    applyNetwork(ctx, OFFLINE);
    expect(ctx.S.network).toEqual({ online: false, since: OFFLINE.since });
  });

  it("follows the network.changed event both ways and ignores other events", () => {
    const ctx = contextOf(createTestMarshal());
    networkSyncer.onEvent?.(ctx, changed(OFFLINE));
    expect(ctx.S.network.online).toBe(false);
    networkSyncer.onEvent?.(ctx, changed(ONLINE));
    expect(ctx.S.network).toEqual({ online: true, since: ONLINE.since });
    networkSyncer.onEvent?.(ctx, {
      topic: "home",
      type: "activity.created",
      data: {},
    } as unknown as WireEvent);
    expect(ctx.S.network.online).toBe(true);
  });

  it("changes nothing when the same answer is applied twice", () => {
    const ctx = contextOf(createTestMarshal());
    applyNetwork(ctx, OFFLINE);
    const first = ctx.S.network;
    applyNetwork(ctx, { ...OFFLINE });
    expect(ctx.S.network).toBe(first);
    expect(ctx.S.network).toEqual({ online: false, since: OFFLINE.since });
  });
});
