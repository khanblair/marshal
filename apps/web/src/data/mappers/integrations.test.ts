import type { GitHubConnect, IntegrationList, TestResult } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "~/data/testing/golden";
import { toGitHubConnection, toIntegrationStates } from "./integrations";
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

// GitHub's sign-in (section S29a). There is no golden for it yet, so the wire answers are built here
// the way the daemon writes them: a field that does not apply to a state is left out.

const EXPIRES = "2026-10-02T18:30:00.000Z";

/** An answer built from loose fields, so a state the types do not name stays writable if they are narrowed. */
const loose = (fields: object): GitHubConnect =>
  ({ installations: [], ...fields }) as unknown as GitHubConnect;

describe("the GitHub connection mapper", () => {
  it("keeps what a pending sign-in carries and turns its expiry into milliseconds", () => {
    const wire: GitHubConnect = {
      state: "pending",
      userCode: "WDJB-MJHT",
      verificationUri: "https://github.com/login/device",
      expiresAt: EXPIRES,
      installations: [],
    };
    expect(toGitHubConnection(wire)).toStrictEqual({
      state: "pending",
      userCode: "WDJB-MJHT",
      verificationUri: "https://github.com/login/device",
      expiresAt: Date.parse(EXPIRES),
      installations: [],
    });
  });

  it("leaves out every field the daemon left out, so a screen asks for what its state has", () => {
    const connection = toGitHubConnection({ state: "idle", installations: [] });
    expect(connection).toStrictEqual({ state: "idle", installations: [] });
    expect(Object.keys(connection)).toEqual(["state", "installations"]);
  });

  it("maps a sign-in that is waiting for the App to be installed", () => {
    expect(
      toGitHubConnection({
        state: "needs_install",
        login: "ada",
        installUrl: "https://github.com/apps/marshal-kanban/installations/new",
        installations: [],
      }),
    ).toStrictEqual({
      state: "needs_install",
      login: "ada",
      installUrl: "https://github.com/apps/marshal-kanban/installations/new",
      installations: [],
    });
  });

  it("maps a connected sign-in with its installations, narrowing each kind", () => {
    const connection = toGitHubConnection({
      state: "connected",
      mode: "oauth",
      login: "ada",
      installUrl: "https://github.com/apps/marshal-kanban/installations/new",
      installations: [
        { account: "ada", kind: "user", allRepositories: true },
        { account: "acme", kind: "organization", allRepositories: false },
        { account: "odd", kind: "enterprise", allRepositories: false },
      ],
    });
    expect(connection.mode).toBe("oauth");
    expect(connection.installations).toEqual([
      { account: "ada", kind: "user", allRepositories: true },
      { account: "acme", kind: "organization", allRepositories: false },
      // A kind this app does not draw differently reads as a user, the plainer of the two.
      { account: "odd", kind: "user", allRepositories: false },
    ]);
  });

  it("maps a connection made by a token, which has no installations", () => {
    expect(
      toGitHubConnection({ state: "connected", mode: "token", login: "ada", installations: [] }),
    ).toStrictEqual({ state: "connected", mode: "token", login: "ada", installations: [] });
  });

  it("keeps the daemon's sentence for a sign-in that ended without connecting", () => {
    for (const state of ["denied", "expired", "failed"]) {
      const connection = toGitHubConnection(loose({ state, message: "GitHub said no." }));
      expect(connection).toMatchObject({ state, message: "GitHub said no." });
    }
  });

  it("reads a state or a mode it does not know without guessing", () => {
    const unknown = toGitHubConnection(loose({ state: "mystery", mode: "magic" }));
    // A failure with a sentence of its own, never one of the states the screen draws a panel for.
    expect(unknown.state).toBe("failed");
    expect(unknown.message).toBe("Marshal got an answer from GitHub it does not know how to show.");
    expect(unknown.mode).toBeUndefined();
    // When the daemon wrote a sentence, that is the one that is kept.
    expect(toGitHubConnection(loose({ state: "mystery", message: "Try later." })).message).toBe(
      "Try later.",
    );
  });

  it("copies the installations, so one answer can be kept and reused", () => {
    const wire: GitHubConnect = {
      state: "connected",
      mode: "oauth",
      installations: [{ account: "ada", kind: "user", allRepositories: true }],
    };
    const connection = toGitHubConnection(wire);
    wire.installations[0]!.account = "changed";
    expect(connection.installations[0]?.account).toBe("ada");
  });

  it("refuses a timestamp it cannot read, as every other mapper does", () => {
    expect(() =>
      toGitHubConnection({ state: "pending", expiresAt: "soon", installations: [] }),
    ).toThrow(RangeError);
  });
});
