import type { IntegrationList } from "@marshal/protocol";
import { unwrap } from "solid-js/store";
import { afterEach, describe, expect, it } from "vitest";
import type { ApiClient } from "~/data/api-client";
import { toIntegrationStates } from "~/data/mappers/integrations";
import { sectionStatus } from "~/data/sections";
import { golden } from "~/data/testing/golden";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { contextOf, createSyncedMarshal, createTestMarshal } from "~/testing/test-store";
import {
  applyIntegrationList,
  applyIntegrationStates,
  connectionOnDaemon,
  integrationsSyncer,
} from "./integrations";

// Section S29a: the GitHub row in Settings. The daemon lists every connection it knows, whether or
// not it is set up, and answers that whole list to a read, a save, a remove, and a test alike, so the
// syncer publishes no topic and its snapshot is the same answer a save gets back.

const list = golden<IntegrationList>("integration-list");
/** A pasted token, as a person types it. It is synthetic: the fake daemon never reaches GitHub. */
const TOKEN = "ghp_synthetic0token0for0tests";
const TOKEN_BODY = { token: TOKEN };

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

const row = (M: ReturnType<typeof createTestMarshal>, id: string) => {
  const found = M.S.integrations.find((integration) => integration.id === id);
  if (!found) throw new Error(`the store has no ${id} connection`);
  return found;
};

describe("the integrations section", () => {
  it("is section S29a, follows no topic, and loads the daemon's whole list", async () => {
    expect(integrationsSyncer.section).toBe("S29a");
    expect(integrationsSyncer.topics).toEqual([]);
    const asked: unknown[] = [];
    const api = {
      listIntegrations: async (options?: unknown) => {
        asked.push(options);
        return list;
      },
    } as unknown as ApiClient;
    expect(await integrationsSyncer.load(api, contextOf(createTestMarshal()))).toEqual(
      toIntegrationStates(list),
    );
    // No project and no arguments: a connection is the daemon's, not one project's.
    expect(asked).toEqual([undefined]);
  });

  it("writes the daemon's half of the GitHub row and leaves the app's own words", () => {
    const ctx = contextOf(createTestMarshal());
    const github = () => ctx.S.integrations.find((integration) => integration.id === "github");
    // The prototype's own "installed on 3 repositories" sentence is emptied once S29a is the
    // daemon's, so nothing shows for a connection nothing is stored for; the name and icon stay.
    expect(unwrap(github())).toMatchObject({
      name: "GitHub",
      icon: "github",
      st: "none",
      detail: "",
    });
    applyIntegrationList(ctx, list);
    // The status, the sentence, and the test are the daemon's now. Only those three moved.
    expect(unwrap(github())).toMatchObject({
      id: "github",
      name: "GitHub",
      icon: "github",
      st: "connected",
      detail: "GitHub App installed on 3 repositories",
    });
    expect(github()?.lastTest?.ok).toBe(true);
  });

  it("leaves a connection whose section is still the mock's exactly as the seed made it", () => {
    const ctx = contextOf(createTestMarshal({ sections: { ...sectionStatus, S29c: "mock" } }));
    const trello = () => ctx.S.integrations.find((integration) => integration.id === "trello");
    const before = unwrap(trello());
    applyIntegrationList(ctx, list);
    // The daemon's answer names a "trello" row too, but with S29c pinned to the mock here, its
    // mock row keeps the sentence the seed gave it.
    expect(unwrap(trello())).toEqual(before);
    expect(trello()?.st).toBe("connected");
  });

  it("writes nothing to a store whose S29a is still the mock's", () => {
    const ctx = contextOf(createTestMarshal({ sections: { ...sectionStatus, S29a: "mock" } }));
    applyIntegrationStates(ctx, toIntegrationStates(list));
    // The row keeps the seed's own status and sentence, not the daemon's answer.
    expect(ctx.S.integrations[0]?.st).toBe("connected");
    expect(ctx.S.integrations[0]?.detail).toBe("GitHub App installed on 3 repositories");
  });

  it("copies what the daemon answered, so one answer can be kept and reused", () => {
    const ctx = contextOf(createTestMarshal());
    const answer = structuredClone(list);
    applyIntegrationStates(ctx, toIntegrationStates(answer));
    answer.integrations[0]!.detail = "Changed later";
    answer.integrations[0]!.lastTest!.checks[0]!.message = "Changed later";
    expect(ctx.S.integrations[0]?.detail).toBe("GitHub App installed on 3 repositories");
    expect(ctx.S.integrations[0]?.lastTest?.checks[0]?.message).toBe(
      "The GitHub App is installed.",
    );
  });

  it("says which connection's row the daemon owns, and which is still the mock's", () => {
    const daemonStore = createTestMarshal();
    expect(connectionOnDaemon(contextOf(daemonStore), "github")).toBe(true);
    expect(connectionOnDaemon(contextOf(daemonStore), "obsidian")).toBe(true);
    expect(connectionOnDaemon(contextOf(daemonStore), "trello")).toBe(true);
    expect(connectionOnDaemon(contextOf(daemonStore), "gcal")).toBe(true);
    const mockStore = createTestMarshal({ sections: { ...sectionStatus, S29a: "mock" } });
    expect(connectionOnDaemon(contextOf(mockStore), "github")).toBe(false);
    const obsidianMockStore = createTestMarshal({ sections: { ...sectionStatus, S29b: "mock" } });
    expect(connectionOnDaemon(contextOf(obsidianMockStore), "obsidian")).toBe(false);
    // Trello's own section (S29c) pinned back to the mock reads the same way.
    const trelloMockStore = createTestMarshal({ sections: { ...sectionStatus, S29c: "mock" } });
    expect(connectionOnDaemon(contextOf(trelloMockStore), "trello")).toBe(false);
  });
});

describe("the connections with a daemon", () => {
  it("reads the daemon's list on boot", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    expect(d.routes()).toContain("GET /v1/integrations");
    expect(M.S.integrations.map((row) => row.id)).toEqual([
      "github",
      "trello",
      "gcal",
      "gmail",
      "gdrive",
      "gdocs",
      "gsheets",
      "gslides",
      "telegram",
      "discord",
      "ntfy",
      "obsidian",
    ]);
    // Every connection the daemon knows reads as not connected before anything is set up.
    expect(row(M, "github").st).toBe("none");
  });

  it("stores a token and keeps the whole list the save answers", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    expect(await M.saveGitHubToken(TOKEN_BODY)).toEqual({ saved: true });
    expect(d.bodies("PUT /v1/integrations/github/token")).toEqual([TOKEN_BODY]);
    expect(row(M, "github").st).toBe("connected");
    // The row's sentence is the one its own passing test wrote into its Summary check, as the
    // daemon derives it (`integrations.go`'s `detailFor`).
    expect(row(M, "github").detail).toBe("The GitHub App works and Marshal can use it.");
    // The daemon tests a connection it just changed, so the row already carries the last test.
    expect(row(M, "github").lastTest?.ok).toBe(true);
    expect(row(M, "github").lastTest?.checks.map((check) => check.name)).toEqual([
      "Summary",
      "App",
      "Repositories",
      "Permissions",
      "Webhook",
    ]);
  });

  it("never lets a secret come back from the daemon", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    await M.saveGitHubToken(TOKEN_BODY);
    expect(d.bodies("PUT /v1/integrations/github/token")).toEqual([TOKEN_BODY]);
    // What the store holds afterwards carries no token, which never leaves the daemon's keychain:
    // only the status, the sentence, and the last test's checks come back.
    expect(JSON.stringify(M.S.integrations)).not.toContain(TOKEN);
    expect(JSON.stringify(await M.readGitHubConnect())).not.toContain(TOKEN);
    expect(row(M, "github").lastTest?.checks.at(-1)?.message).toBe("A ping reached Marshal.");
  });

  it("answers the daemon's own sentence and keeps what it had when a token is refused", async () => {
    const d = createFakeDaemon({
      integrationSaveRefused: () => "GitHub did not accept that token.",
    });
    daemon = d;
    const M = await createSyncedMarshal(d);
    expect(await M.saveGitHubToken(TOKEN_BODY)).toEqual({
      error: "GitHub did not accept that token.",
    });
    // The sentence goes to the dialog beside the field, not to a toast.
    expect(M.S.toasts).toEqual([]);
    expect(row(M, "github").st).toBe("none");
  });

  it("tests a token without saving anything", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    const result = await M.testGitHubToken(TOKEN_BODY);
    expect(result).toMatchObject({ ok: true });
    expect(d.bodies("POST /v1/integrations/github/token/test")).toEqual([TOKEN_BODY]);
    // Nothing was stored: the row is still not connected and there was no save.
    expect(row(M, "github").st).toBe("none");
    expect(d.routes()).not.toContain("PUT /v1/integrations/github/token");
  });

  it("says what is wrong with a token the test refuses, in a failed check", async () => {
    const d = createFakeDaemon({
      integrationSaveRefused: () => "GitHub did not accept that token.",
    });
    daemon = d;
    const M = await createSyncedMarshal(d);
    const result = await M.testGitHubToken(TOKEN_BODY);
    if ("error" in result) throw new Error("the test ran, so it answers a result");
    expect(result.ok).toBe(false);
    expect(result.checks[0]).toMatchObject({
      name: "Token",
      state: "failed",
      message: "GitHub did not accept that token.",
    });
  });

  it("starts a sign-in and answers the code to type, as a connection the screen can read", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    const answer = await M.startGitHubConnect();
    expect(d.routes()).toContain("POST /v1/integrations/github/connect");
    expect(answer).toMatchObject({
      state: "pending",
      userCode: "WDJB-MJHT",
      verificationUri: "https://github.com/login/device",
      installations: [],
    });
    // The wire's timestamp is milliseconds here, like every other time the screens read.
    expect("error" in answer ? 0 : answer.expiresAt).toBeGreaterThan(Date.now());
  });

  it("reads the sign-in on, and reads the list again the first time it says connected", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    await M.startGitHubConnect();
    d.integrations.github.script.push({
      state: "connected",
      mode: "oauth",
      login: "ada",
      installations: [{ account: "ada", kind: "user", allRepositories: true }],
    });
    const lists = () => d.routes().filter((route) => route === "GET /v1/integrations").length;
    const before = lists();
    expect(row(M, "github").st).toBe("none");
    const answer = await M.readGitHubConnect();
    expect(answer).toMatchObject({ state: "connected", mode: "oauth", login: "ada" });
    // The connect routes answer the sign-in, not the list, so the row needs its own read.
    expect(lists()).toBe(before + 1);
    expect(row(M, "github").st).toBe("connected");
    expect(row(M, "github").lastTest?.ok).toBe(true);
    // The next read finds the row and the daemon agreeing, and asks for nothing more.
    await M.readGitHubConnect();
    expect(lists()).toBe(before + 1);
  });

  it("reads a stored token connection when no sign-in is under way, and nothing before one", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    expect(await M.readGitHubConnect()).toEqual({ state: "idle", installations: [] });
    await M.saveGitHubToken(TOKEN_BODY);
    expect(await M.readGitHubConnect()).toEqual({
      state: "connected",
      mode: "token",
      login: "ada",
      installations: [],
    });
    // A stored sign-in also carries the page that adds another account.
    d.integrations.github.mode = "oauth";
    expect(await M.readGitHubConnect()).toMatchObject({
      mode: "oauth",
      installUrl: "https://github.com/apps/marshal-kanban/installations/new",
      installations: [{ account: "ada", kind: "user", allRepositories: true }],
    });
  });

  it("cancels a pending sign-in", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    await M.startGitHubConnect();
    expect(await M.cancelGitHubConnect()).toMatchObject({ state: "idle" });
    expect(d.routes()).toContain("DELETE /v1/integrations/github/connect");
    expect(await M.readGitHubConnect()).toMatchObject({ state: "idle" });
  });

  it("narrows a state it does not know to a failure with a sentence of its own", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    // Built from text, so the test stays true if the generated state type is ever narrowed to a union.
    d.integrations.github.script.push(JSON.parse('{"state":"mystery","installations":[]}'));
    const answer = await M.readGitHubConnect();
    expect(answer).toMatchObject({ state: "failed" });
    expect("error" in answer ? "" : answer.message).toContain("does not know how to show");
  });

  it("runs a test on the daemon, keeps what it found, and refuses a second one too soon", async () => {
    const d = createFakeDaemon({
      integrations: [
        {
          id: "github",
          kind: "github",
          st: "connected",
          detail: "GitHub App installed on 3 repositories",
        },
      ],
    });
    daemon = d;
    const M = await createSyncedMarshal(d);
    expect(await M.testIntegration("github")).toBe(true);
    expect(row(M, "github").lastTest?.ok).toBe(true);
    expect(row(M, "github").lastTest?.checks.map((check) => check.state)).toEqual([
      "passed",
      "passed",
      "passed",
      "passed",
      "passed",
    ]);
    expect(row(M, "github").detail).toBe("The GitHub App works and Marshal can use it.");
    // The daemon's cooldown holds two tests of one connection apart, and its own sentence says how
    // long to wait.
    expect(await M.testIntegration("github")).toBe(false);
    expect(M.S.toasts.map((toast) => toast.msg)).toEqual([
      "This connection was tested a moment ago. Try again in 5 seconds.",
    ]);
  });

  it("marks the row an error when a test finds something wrong, and keeps the fix it gave", async () => {
    const d = createFakeDaemon({
      integrations: [
        { id: "github", kind: "github", st: "connected", detail: "GitHub App installed." },
      ],
      integrationChecks: () => [
        {
          name: "Permissions",
          state: "failed",
          message: "The App cannot read pull requests.",
          fix: "Grant pull request access to the App on GitHub.",
        },
      ],
    });
    daemon = d;
    const M = await createSyncedMarshal(d);
    // The daemon answered a result, not an error, so the test ran: a failed check reads as "failed".
    expect(await M.testIntegration("github")).toBe(false);
    expect(row(M, "github").st).toBe("error");
    expect(row(M, "github").lastTest?.ok).toBe(false);
    expect(row(M, "github").lastTest?.checks[0]?.fix).toBe(
      "Grant pull request access to the App on GitHub.",
    );
    // The row's sentence is that check's fix, which is the one thing a person can act on.
    expect(row(M, "github").detail).toBe("Grant pull request access to the App on GitHub.");
  });

  it("forgets a connection's settings and answers the whole list back", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    await M.saveGitHubToken(TOKEN_BODY);
    expect(await M.disconnectIntegration("github")).toBe(true);
    expect(d.routes()).toContain("DELETE /v1/integrations/github");
    expect(row(M, "github").st).toBe("none");
    expect(row(M, "github").detail).toBe("");
    expect(row(M, "github").lastTest).toBeUndefined();
  });

  it("refuses a connection the daemon does not know", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    // Only some connections can be saved, so an unknown one has no shape: the daemon refuses it
    // rather than silently accepting it, and the row does not change.
    expect(await M.disconnectIntegration("nope")).toBe(false);
    expect(row(M, "github").st).toBe("none");
  });

  // Section S29b: the Obsidian vault row. Nobody sets this connection up - it is Marshal's own
  // folder, not a person's setting - so it starts connected rather than "none", the way the real
  // daemon's `vaultStatus` always answers once a vault root is set.
  it("reads the Obsidian row connected before anything is tested", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    expect(row(M, "obsidian").st).toBe("connected");
    expect(row(M, "obsidian").lastTest).toBeUndefined();
  });

  it("runs the Obsidian vault's own test and keeps what it found", async () => {
    const d = createFakeDaemon();
    daemon = d;
    const M = await createSyncedMarshal(d);
    expect(await M.testIntegration("obsidian")).toBe(true);
    expect(row(M, "obsidian").st).toBe("connected");
    expect(row(M, "obsidian").lastTest?.ok).toBe(true);
    expect(row(M, "obsidian").lastTest?.checks.map((check) => check.name)).toEqual([
      "Summary",
      "Vault folder",
      "Vault writable",
    ]);
  });

  it("marks the Obsidian row needing attention when its test finds the vault unwritable", async () => {
    const d = createFakeDaemon({
      integrationChecks: (r) =>
        r.id === "obsidian"
          ? [
              {
                name: "Vault writable",
                state: "failed",
                message: "The vault folder is read-only, so Marshal cannot save notes in it.",
                fix: "Give your user write access to the vault folder, then test again.",
              },
            ]
          : [],
    });
    daemon = d;
    const M = await createSyncedMarshal(d);
    expect(await M.testIntegration("obsidian")).toBe(false);
    expect(row(M, "obsidian").st).toBe("error");
    expect(row(M, "obsidian").lastTest?.ok).toBe(false);
    expect(row(M, "obsidian").detail).toBe(
      "Give your user write access to the vault folder, then test again.",
    );
  });
});

describe("the connections with no daemon", () => {
  it("changes nothing and shows nothing, because there is nothing to ask", async () => {
    const M = createTestMarshal();
    const refused = { error: "Marshal is not connected to its daemon." };
    expect(await M.saveGitHubToken(TOKEN_BODY)).toEqual(refused);
    expect(await M.testGitHubToken(TOKEN_BODY)).toEqual(refused);
    expect(await M.startGitHubConnect()).toEqual(refused);
    expect(await M.readGitHubConnect()).toEqual(refused);
    expect(await M.cancelGitHubConnect()).toEqual(refused);
    expect(await M.disconnectIntegration("github")).toBe(false);
    expect(await M.testIntegration("github")).toBe(false);
    expect(M.S.toasts).toEqual([]);
    expect(row(M, "github").st).toBe("none");
  });
});
