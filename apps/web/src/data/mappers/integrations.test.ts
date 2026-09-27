import type { IntegrationList, TestResult } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "~/data/testing/golden";
import { toIntegrationStates } from "./integrations";
import { toProviderTest } from "./providers";

// Section S29a: the GitHub row in Settings. The mapper is tested from the golden the daemon's own
// contract test writes (`daemon/testdata/golden/integration-list.json`), so the two sides cannot
// drift: the same file is the answer GET /v1/integrations and every change route answer with.

const list = golden<IntegrationList>("integration-list");

describe("the integration mapper", () => {
  it("keeps the daemon's order and each connection's id, status, and sentence", () => {
    const states = toIntegrationStates(list);
    expect(states.map((state) => state.id)).toEqual(["github", "trello", "calendar"]);
    expect(states.map((state) => state.st)).toEqual(["connected", "none", "error"]);
    expect(states[0]?.detail).toBe("GitHub App installed on 3 repositories");
    // A connection nothing is stored for carries no sentence: the screen writes its own words there.
    expect(states[1]?.detail).toBe("");
  });

  it("maps the last test's checks, fix hints and all", () => {
    const [github, trello, calendar] = toIntegrationStates(list);
    expect(github?.lastTest?.ok).toBe(true);
    expect(github?.lastTest?.checks.map((check) => check.name)).toEqual([
      "App",
      "Repositories",
      "Permissions",
      "Webhook",
    ]);
    expect(github?.lastTest?.checks.every((check) => check.state === "passed")).toBe(true);
    // A connection that was never tested carries no result, so the screen shows no checks at all.
    expect(trello?.lastTest).toBeUndefined();
    // A failed check keeps its fix hint, which is what tells a person what to do about it.
    expect(calendar?.lastTest?.ok).toBe(false);
    expect(calendar?.lastTest?.checks[0]?.state).toBe("failed");
    expect(calendar?.lastTest?.checks[0]?.fix).toBe("Connect the calendar again in Settings.");
  });

  it("copies what the daemon answered, so one answer can be kept and reused", () => {
    const answer = structuredClone(list);
    const states = toIntegrationStates(answer);
    answer.integrations[0]!.detail = "Changed later";
    expect(states[0]?.detail).toBe("GitHub App installed on 3 repositories");
  });

  // Section S29b: the Obsidian vault row. A connection's test is the same shape a provider's is
  // (docs/architecture.md section 18, IntegrationTest = ProviderTest), so the Obsidian vault's own
  // result golden (`daemon/testdata/golden/test-result-obsidian.json`, written by
  // internal/protocol's TestTestResultObsidianGolden) is read by the same mapper GitHub's test uses.
  it("reads the Obsidian vault's own test result the same way a provider's is read", () => {
    const result = golden<TestResult>("test-result-obsidian");
    const test = toProviderTest(result);
    expect(test.ok).toBe(true);
    expect(test.checks.map((check) => check.name)).toEqual([
      "Summary",
      "Vault folder",
      "Vault writable",
    ]);
    expect(test.checks.every((check) => check.state === "passed")).toBe(true);
  });
});
