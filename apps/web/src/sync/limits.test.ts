import type { LimitList } from "@marshal/protocol";
import { unwrap } from "solid-js/store";
import { describe, expect, it } from "vitest";
import type { ApiClient } from "~/data/api-client";
import { golden } from "~/data/testing/golden";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { applyLimitList, limitsSyncer } from "./limits";

// Section S26b: the cost and awake limits. The daemon holds them, the routes all answer the whole
// list, and the syncer publishes no topic, so its snapshot is the same answer a save gets back.

const list = golden<LimitList>("limit-list");

/** The store's shape of that list, which is what every assertion below expects. */
const expected = {
  global: { day: 25, month: 400, awake: 18 },
  "01JD7Q4M2X8K9V0P5T3RB6NHAE": { day: 5 },
};

describe("the limits section", () => {
  it("is section S26b, follows no topic, and loads the daemon's list", async () => {
    expect(limitsSyncer.section).toBe("S26b");
    expect(limitsSyncer.topics).toEqual([]);
    const asked: string[] = [];
    const api = {
      listLimits: async () => {
        asked.push("listLimits");
        return list;
      },
    } as unknown as ApiClient;
    expect(await limitsSyncer.load(api, contextOf(createTestMarshal()))).toEqual(expected);
    expect(asked).toEqual(["listLimits"]);
  });

  it("puts the daemon's whole list in the store, replacing the ceilings it had", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.limits = { global: { day: 1 } };
    limitsSyncer.apply(ctx, expected);
    expect(unwrap(ctx.S.limits)).toEqual(expected);
  });

  it("keeps only global when the daemon answers with no ceilings, and drops an old one", () => {
    const ctx = contextOf(createTestMarshal());
    applyLimitList(ctx, list);
    applyLimitList(ctx, { limits: [] });
    expect(ctx.S.limits).toEqual({ global: {} });
  });
});
