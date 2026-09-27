import { describe, expect, it } from "vitest";
import type { Limit, LimitList, Provider, ProviderList, SaveProviderRequest } from "../src";
import { golden } from "./golden";

// Provider keys and the ceilings on what they may cost (docs/backend-checklist.md B4.4 and B4.5,
// inventory N18, tasks 4.1/4.8/4.9). The samples are checked against the generated types by the
// compiler and against the daemon's own files by the assertions.
describe("the provider list and the limits", () => {
  /** One provider row, with the fields a screen reads for the common case. */
  const row = (
    id: string,
    name: string,
    st: Provider["st"],
    masked: string,
    models: string,
  ): Provider => ({ id, name, st, masked, models, error: "", local: false });

  it("has a row per status, one of them local, and the server's time", () => {
    const sample: ProviderList = {
      providers: [
        row("anthropic", "Anthropic", "saved", "sk-ant-…4f2a", "Claude models"),
        row("deepseek", "DeepSeek", "empty", "", "DeepSeek models"),
        {
          ...row("openrouter", "OpenRouter", "invalid", "sk-or-…0b33", "Any model on OpenRouter"),
          error:
            "OpenRouter rejected this key. Create a new key at openrouter.ai/keys and paste it here.",
        },
        {
          ...row("ollama", "Ollama", "saved", "http://localhost:11434", "Local models"),
          local: true,
        },
      ],
      serverTime: "2026-09-27T09:30:00.000Z",
    };
    expect(golden("provider-list")).toEqual(sample);
    // The key itself is never on the wire: the row carries a masked value and nothing else, so a
    // screen cannot show a key it was not given. A local provider's "key" field holds a URL, which
    // is not a secret and is shown as it was typed.
    const anthropic = sample.providers[0];
    expect(anthropic?.masked).toContain("…");
    expect(anthropic?.masked).not.toBe("sk-ant-4f2a");
    expect(sample.providers[3]?.local).toBe(true);
    // Only the refused key carries a sentence telling the person what to do about it.
    expect(sample.providers.map((provider) => provider.error !== "")).toEqual([
      false,
      false,
      true,
      false,
    ]);
  });

  it("has the ceilings that are set, a global trio and one project's daily cost", () => {
    const sample: LimitList = {
      limits: [
        { scope: "global", kind: "cost-day", value: 25_000_000 },
        { scope: "global", kind: "cost-month", value: 400_000_000 },
        { scope: "global", kind: "awake", value: 18 },
        { scope: "01JD7Q4M2X8K9V0P5T3RB6NHAE", kind: "cost-day", value: 5_000_000 },
      ],
    };
    expect(golden("limit-list")).toEqual(sample);
    // Cost is micro-dollars, the unit the rest of the daemon uses for money: 25_000_000 is $25.
    const cost = sample.limits.find(
      (limit) => limit.kind === "cost-day" && limit.scope === "global",
    );
    expect((cost?.value ?? 0) / 1_000_000).toBe(25);
    // Awake counts cards, not milliseconds: 18 is eighteen awake cards, the number the prototype's
    // global limit shows (apps/web/src/mock/state.ts seeds awake: 18).
    const awake = sample.limits.find((limit) => limit.kind === "awake");
    expect(awake?.value).toBe(18);
  });

  it("sends only the key when a key is saved, and the scope and kind in the path", () => {
    const body: SaveProviderRequest = { key: "sk-ant-a-new-key" };
    expect(Object.keys(body)).toEqual(["key"]);
    // A limit's scope and kind are the path's, so the body is only the number, which is what makes
    // the same request shape work for a global limit and for one project's.
    const limit: Limit = { scope: "global", kind: "awake", value: 18 };
    expect(limit.kind satisfies string).toBe("awake");
  });
});
