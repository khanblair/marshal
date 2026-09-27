import { describe, expect, it } from "vitest";
import type {
  CISnapshot,
  CiRun,
  IntegrationList,
  LocalCIResult,
  Preview,
  PreviewShotResult,
  PreviewSnapshot,
  SaveGitHubRequest,
  SimulateCIFailureRequest,
  SimulateCIFailureResult,
} from "../src";
import { golden } from "./golden";

// The CI, preview, and connection samples, checked against the generated types by the compiler and
// against the Go goldens at run time, the same way the other golden files are.

const CARD_ID = "01M3C107JB041061050R3GG28A";

const failedRun: CiRun = {
  id: "01JD7Q4M2X8K9V0P5T3RB6NHC1",
  projectId: "web-dashboard",
  cardId: CARD_ID,
  branch: "marshal/card-12-fix-the-report",
  workflow: "ci",
  status: "failed",
  url: "https://github.com/khanblair/web-dashboard/actions/runs/9931",
  startedAt: "2026-09-27T09:26:00.000Z",
  updatedAt: "2026-09-27T09:30:00.000Z",
};

describe("CI golden files", () => {
  it("has a run whose start time is null while it is queued", () => {
    expect(golden("ci-run")).toEqual(failedRun);
  });

  it("has a project's CI health and its runs", () => {
    const started = "2026-09-27T08:48:00.000Z";
    const sample: CISnapshot = {
      projects: [
        {
          projectId: "web-dashboard",
          status: "passed",
          runs: [
            {
              id: "01JD7Q4M2X8K9V0P5T3RB6NHC3",
              projectId: "web-dashboard",
              cardId: CARD_ID,
              branch: "marshal/card-12-fix-the-report",
              workflow: "lint",
              status: "queued",
              url: "https://github.com/khanblair/web-dashboard/actions/runs/9932",
              startedAt: null,
              updatedAt: "2026-09-27T09:30:00.000Z",
            },
            failedRun,
            {
              id: "01JD7Q4M2X8K9V0P5T3RB6NHC2",
              projectId: "web-dashboard",
              cardId: "",
              branch: "main",
              workflow: "ci packages/web",
              status: "passed",
              url: "https://github.com/khanblair/web-dashboard/actions/runs/9928",
              startedAt: started,
              updatedAt: "2026-09-27T08:54:00.000Z",
            },
          ],
        },
      ],
      serverTime: "2026-09-27T09:30:00.000Z",
    };
    expect(golden("ci-snapshot")).toEqual(sample);
  });

  it("has both Simulate CI failure modes, as a request and as an answer", () => {
    const request: SimulateCIFailureRequest = { mode: "synthetic" };
    expect(golden("simulate-ci-failure-request")).toEqual(request);
    const result: SimulateCIFailureResult = {
      cardId: CARD_ID,
      mode: "synthetic",
      run: failedRun,
      fixStarted: true,
      commit: "",
    };
    expect(golden("simulate-ci-failure-result")).toEqual(result);
  });
});

describe("local CI golden files", () => {
  it("has a run with every step shape: ran, not reached, passed, and refused", () => {
    const sample: LocalCIResult = {
      cardId: CARD_ID,
      branch: "marshal/card-12-fix-the-report",
      workflows: [
        {
          file: ".github/workflows/ci.yml",
          name: "CI",
          status: "failed",
          steps: [
            {
              job: "test",
              name: "Run the tests",
              kind: "test",
              status: "failed",
              reason: "",
              command: "pnpm install && pnpm test",
              output: "FAIL src/util.test.ts\n  add() adds two numbers\n  1 test failed",
              tookMs: 4210,
            },
            {
              job: "test",
              name: "Typecheck",
              kind: "lint",
              status: "skipped",
              reason: "An earlier step in this job failed, so this step did not run.",
              command: "pnpm exec tsc --noEmit",
              output: "",
              tookMs: 0,
            },
            {
              job: "build",
              name: "Build the packages",
              kind: "build",
              status: "passed",
              reason: "",
              command: "pnpm build",
              output: "built in 3.1s",
              tookMs: 3120,
            },
            {
              job: "release",
              name: "Publish to npm",
              kind: "other",
              status: "unsupported",
              reason: "This step deploys or publishes something, so Marshal did not run it here.",
              command: "pnpm publish --access public",
              output: "",
              tookMs: 0,
            },
          ],
        },
      ],
      serverTime: "2026-09-27T09:30:00.000Z",
    };
    expect(golden("local-ci-result")).toEqual(sample);
  });
});

describe("preview golden files", () => {
  const running: Preview = {
    cardId: CARD_ID,
    state: "running",
    url: "http://127.0.0.1:5112/reports",
    port: 5112,
    command: "pnpm dev",
    startedAt: "2026-09-27T09:28:30.000Z",
    error: "",
    shots: [
      {
        kind: "before",
        url: `/v1/cards/${CARD_ID}/preview/shots/before.png?v=1790488200000`,
        width: 1440,
        height: 900,
        takenAt: "2026-09-27T09:29:30.000Z",
      },
      {
        kind: "after",
        url: `/v1/cards/${CARD_ID}/preview/shots/after.png?v=1790488260000`,
        width: 1440,
        height: 900,
        takenAt: "2026-09-27T09:30:00.000Z",
      },
    ],
  };

  it("has a running preview with its address and both screenshots", () => {
    expect(golden("preview")).toEqual(running);
  });

  it("has the same preview with the daemon's time beside it", () => {
    const sample: PreviewSnapshot = {
      preview: running,
      serverTime: "2026-09-27T09:30:00.000Z",
    };
    expect(golden("preview-snapshot")).toEqual(sample);
  });

  it("has a screenshot answer that says what was captured", () => {
    const sample: PreviewShotResult = {
      preview: running,
      outcome: "taken",
      notice: "The screenshot was taken.",
      serverTime: "2026-09-27T09:30:00.000Z",
    };
    expect(golden("preview-shot-result")).toEqual(sample);
  });
});

describe("connection golden files", () => {
  it("has the three connection states and a test result with its checks", () => {
    const sample: IntegrationList = {
      integrations: [
        {
          id: "github",
          kind: "github",
          st: "connected",
          detail: "GitHub App installed on 3 repositories",
          lastTest: {
            connectionId: "github",
            checks: [
              { name: "App", state: "passed", message: "The GitHub App is installed." },
              { name: "Repositories", state: "passed", message: "3 repositories are visible." },
              {
                name: "Permissions",
                state: "passed",
                message: "Issues, pull requests, and Actions are allowed.",
              },
              { name: "Webhook", state: "passed", message: "A ping reached Marshal." },
            ],
            ok: true,
            ranAt: "2026-09-27T09:30:00.000Z",
          },
        },
        { id: "trello", kind: "trello", st: "none", detail: "" },
        {
          id: "calendar",
          kind: "calendar",
          st: "error",
          detail: "The Google Calendar connection needs attention",
          lastTest: {
            connectionId: "calendar",
            checks: [
              {
                name: "Token",
                state: "failed",
                message: "Google refused this connection's token.",
                fix: "Connect the calendar again in Settings.",
              },
            ],
            ok: false,
            ranAt: "2026-09-27T07:30:00.000Z",
          },
        },
      ],
      serverTime: "2026-09-27T09:30:00.000Z",
    };
    expect(golden("integration-list")).toEqual(sample);
  });

  it("never carries a key or a webhook secret back from a save", () => {
    // The request carries them; no answer type has a field for either, so a route cannot send one
    // by accident.
    const request: SaveGitHubRequest = {
      appId: 1282340,
      installationId: 55123907,
      privateKey:
        "-----BEGIN RSA PRIVATE KEY-----\nMIIEogIBAAKCAQEA…\n-----END RSA PRIVATE KEY-----\n",
      webhookSecret: "a-synthetic-webhook-secret",
    };
    expect(golden("save-github-request")).toEqual(request);
  });
});
