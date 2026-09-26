import { afterEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import { agentOptions } from "~/mock/agents";
import { GOLDEN_CATALOG, PROTOTYPE_CATALOG, wireAgent, wireCatalog } from "~/testing/agents";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { contextOf, createSyncedMarshal, createTestMarshal } from "~/testing/test-store";
import { agentsSyncer, applyAgentCatalog, BUILT_IN_AGENT, withBuiltIn } from "./agents";

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});
const open = (options?: Parameters<typeof createFakeDaemon>[0]): FakeDaemon => {
  daemon = createFakeDaemon(options);
  return daemon;
};
const names = (M: { AGENTS: Record<string, unknown> }) => Object.keys(M.AGENTS);

describe("applyAgentCatalog", () => {
  it("mirrors the daemon's agents into the store, in the daemon's order, and nothing else of the answer", () => {
    const ctx = contextOf(createTestMarshal());
    applyAgentCatalog(ctx, GOLDEN_CATALOG);
    expect(ctx.S.agents.map((agent) => agent.name)).toEqual(["Claude Code", "Gemini CLI", "Codex"]);
    expect(ctx.S.agents[2]).toMatchObject({ status: "missing", version: "" });
    expect(JSON.stringify(ctx.S.agents)).not.toContain(GOLDEN_CATALOG.serverTime);
  });

  it("changes nothing when the same catalog is applied twice, even with another server time", () => {
    const ctx = contextOf(createTestMarshal());
    applyAgentCatalog(ctx, GOLDEN_CATALOG);
    const first = ctx.S.agents;
    applyAgentCatalog(ctx, { ...GOLDEN_CATALOG, serverTime: "2030-01-01T00:00:00.000Z" });
    expect(ctx.S.agents).toBe(first);
  });

  it("replaces the list when the daemon's catalog changed, such as an agent that was installed", () => {
    const ctx = contextOf(createTestMarshal());
    applyAgentCatalog(ctx, GOLDEN_CATALOG);
    const installed = wireAgent({
      kind: "codex",
      name: "Codex",
      status: "supported",
      version: "0.42.0",
    });
    applyAgentCatalog(ctx, wireCatalog([...GOLDEN_CATALOG.agents.slice(0, 2), installed]));
    expect(ctx.S.agents[2]).toMatchObject({ status: "supported", version: "0.42.0" });
  });

  it("keeps its own copy, so a change to the wire object does not reach the store", () => {
    const ctx = contextOf(createTestMarshal());
    const wire = structuredClone(GOLDEN_CATALOG);
    applyAgentCatalog(ctx, wire);
    wire.agents[0]?.models.push({ id: "extra", name: "Extra", thinking: false });
    expect(ctx.S.agents[0]?.models).toHaveLength(2);
  });

  it("accepts an empty catalog", () => {
    const ctx = contextOf(createTestMarshal());
    applyAgentCatalog(ctx, wireCatalog([]));
    expect(ctx.S.agents).toEqual([]);
    expect(agentOptions(ctx).map((agent) => agent.name)).toEqual(["Built-in agent"]);
  });
});

describe("the built-in agent", () => {
  it("is added after the daemon's agents and is the only entry the app makes up", () => {
    const listed = withBuiltIn(GOLDEN_CATALOG.agents).agents;
    expect(listed.map((agent) => agent.name)).toEqual([
      "Claude Code",
      "Gemini CLI",
      "Codex",
      "Built-in agent",
    ]);
    expect(listed.at(-1)).toBe(BUILT_IN_AGENT);
  });

  it("is not added a second time once the daemon lists one", () => {
    const own = wireAgent({ kind: "builtin", name: "Built-in agent", version: "1.0" });
    expect(
      withBuiltIn([...GOLDEN_CATALOG.agents, own]).agents.filter((a) => a.kind === "builtin"),
    ).toEqual([own]);
  });

  it("gives deepseek-chat and qwen2.5-coder:32b no thinking setting, like the prototype's fixed list", () => {
    const off = BUILT_IN_AGENT.models.filter((model) => !model.thinking).map((model) => model.id);
    expect(off).toEqual(["deepseek-chat", "qwen2.5-coder:32b"]);
  });
});

describe("the agents section", () => {
  it("is section S4 with no event topics and no events of its own", () => {
    expect(agentsSyncer.section).toBe("S4");
    expect(agentsSyncer.topics).toEqual([]);
    expect(agentsSyncer.onEvent).toBeUndefined();
    expect(sectionStatus.S4).toBe("daemon");
  });

  it("loads the catalog from the daemon on connect, before the app is ready", async () => {
    const d = open();
    const release = d.holdNext("GET /v1/agents");
    const M = createTestMarshal({ data: d.data });
    await d.connect();
    await vi.waitFor(() => expect(d.routes()).toContain("GET /v1/projects"));
    // The projects are in, but the catalog is not, so the app frame does not draw yet.
    await vi.waitFor(() => expect(d.routes()).toContain("GET /v1/agents"));
    expect(M.S.ready).toBe(false);
    expect(M.S.agents).toEqual([]);
    release();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    expect(names(M)).toEqual(["Claude Code", "Gemini CLI", "Codex", "Built-in agent"]);
  });

  it("subscribes to nothing for it: the catalog is not a stream", async () => {
    const d = open();
    await createSyncedMarshal(d);
    // Only the Home topic and the person's own (`me`): nothing is named for the catalog.
    expect(d.sockets.last().hellos()[0]?.subscribe).toEqual(["home", "me"]);
  });

  it("shows the first-load error, not the app, when the catalog cannot be loaded, and loads it on Try again", async () => {
    const d = open();
    d.refuseNext("GET /v1/agents", 500, "internal", "Marshal ran into a problem. Try again.");
    const M = createTestMarshal({ data: d.data });
    await d.connect();
    await vi.waitFor(() => expect(M.S.loadError).toBe("Marshal ran into a problem. Try again."));
    expect(M.S.ready).toBe(false);
    M.reconnect();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    expect(M.S.loadError).toBe("");
    expect(M.S.agents).toHaveLength(3);
  });

  it("loads it again when the stream says events were missed, so an agent installed meanwhile shows", async () => {
    const d = open();
    const M = await createSyncedMarshal(d);
    expect(M.AGENTS.Codex?.version).toBe("");
    d.catalog.agents = PROTOTYPE_CATALOG.agents;
    d.sockets.last().push({
      type: "resync",
      epoch: "01M3C0ZZZZ000000000000000C",
      reason: "epoch-changed",
      seq: 0,
    });
    await vi.waitFor(() => expect(M.AGENTS.Codex?.version).toBe("0.42.0"));
  });

  it("is skipped while the section is still mock, and the store then has no catalog of its own", async () => {
    const d = open();
    const M = createTestMarshal({ data: d.data, sections: { ...sectionStatus, S4: "mock" } });
    await d.connect();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    expect(d.routes()).not.toContain("GET /v1/agents");
    expect(M.S.agents).toEqual([]);
  });
});
