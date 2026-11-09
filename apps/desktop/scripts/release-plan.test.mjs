import assert from "node:assert/strict";
import { test } from "node:test";
import { planRelease } from "./release-plan.mjs";

const SHA = "a".repeat(40);
const REPOSITORY = { owner: "owner", repo: "marshal" };
function missing() {
  throw Object.assign(new Error("Not found"), { status: 404 });
}
/** What GitHub really answers when a commit lookup names a tag that does not exist yet. */
function noCommit() {
  throw Object.assign(new Error("No commit found for SHA: v0.1.3"), { status: 422 });
}
function fixture(options = {}) {
  const run = {
    name: "ci",
    conclusion: "success",
    event: "push",
    head_branch: "main",
    head_sha: SHA,
    head_repository: { full_name: "owner/marshal" },
    ...options.run,
  };
  return {
    context: {
      eventName: options.manual ? "workflow_dispatch" : "workflow_run",
      repo: REPOSITORY,
      payload: { workflow_run: run },
    },
    tag: "v0.1.3",
    target: options.target ?? "all",
    github: {
      rest: {
        repos: {
          getBranch: async () => ({ data: { commit: { sha: options.branchSha ?? SHA } } }),
          getContent: async () => ({
            data: {
              content: Buffer.from(
                JSON.stringify({ version: options.version ?? "0.1.3" }),
              ).toString("base64"),
            },
          }),
          getReleaseByTag: async () => (options.release ? { data: options.release } : missing()),
          getCommit: async () => (options.tagSha ? { data: { sha: options.tagSha } } : noCommit()),
        },
        actions: {
          listWorkflowRuns: async () => ({ data: { workflow_runs: options.runs ?? [run] } }),
        },
      },
    },
  };
}

test("successful latest main CI selects both platforms at its exact commit", async () => {
  const plan = await planRelease(fixture());
  assert.deepEqual(plan, { ready: true, sha: SHA, tag: "v0.1.3", target: "all", newRelease: true });
});

test("a version whose tag does not exist yet is ready, but other 422 answers still stop release", async () => {
  assert.equal((await planRelease(fixture())).ready, true);
  const broken = fixture();
  broken.github.rest.repos.getCommit = async () => {
    throw Object.assign(new Error("Validation Failed"), { status: 422 });
  };
  await assert.rejects(planRelease(broken), /Validation Failed/);
});

test("failed CI, pull requests, and foreign repositories never authorize release", async () => {
  for (const run of [
    { conclusion: "failure" },
    { event: "pull_request" },
    { head_branch: "feature" },
    { head_repository: { full_name: "fork/marshal" } },
    { name: "another-workflow" },
  ]) {
    await assert.rejects(planRelease(fixture({ run })), /successful main/);
  }
});

test("superseded commits and already published versions are skipped", async () => {
  assert.equal((await planRelease(fixture({ branchSha: "b".repeat(40) }))).ready, false);
  assert.equal((await planRelease(fixture({ release: { draft: false } }))).ready, false);
});

test("new versions require both platforms and never move an existing tag", async () => {
  await assert.rejects(planRelease(fixture({ target: "android" })), /both desktop and Android/);
  await assert.rejects(planRelease(fixture({ tagSha: "b".repeat(40) })), /never move/);
  assert.equal((await planRelease(fixture({ release: { draft: true }, tagSha: SHA }))).ready, true);
});

test("manual retries require successful push CI on the exact source commit", async () => {
  await assert.rejects(
    planRelease(fixture({ manual: true, tagSha: SHA, runs: [] })),
    /no successful main CI/,
  );
  await assert.rejects(
    planRelease(
      fixture({
        manual: true,
        tagSha: SHA,
        runs: [
          { head_sha: "b".repeat(40), head_branch: "main", event: "push", conclusion: "success" },
        ],
      }),
    ),
    /no successful main CI/,
  );
  const plan = await planRelease(
    fixture({ manual: true, tagSha: SHA, target: "android", release: { draft: false } }),
  );
  assert.equal(plan.newRelease, false);
  assert.equal(plan.target, "android");
  assert.equal(plan.sha, SHA);
});

test("API errors and invalid configuration fail rather than publishing unverified code", async () => {
  const broken = fixture();
  broken.github.rest.repos.getReleaseByTag = async () => {
    throw Object.assign(new Error("Forbidden"), { status: 403 });
  };
  await assert.rejects(planRelease(broken), /Forbidden/);
  await assert.rejects(planRelease(fixture({ version: "not a version" })), /version/);
  await assert.rejects(planRelease(fixture({ target: "unknown" })), /Unknown release target/);
});
