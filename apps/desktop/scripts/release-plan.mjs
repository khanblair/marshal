import { parseReleaseTag } from "./prepare-release.mjs";

/** Only a missing GitHub object is optional. Authentication and API failures must stop release. */
async function optional(read) {
  try {
    return (await read()).data;
  } catch (error) {
    if (error.status === 404) return null;
    throw error;
  }
}

function trustedCompletion(context) {
  const run = context.payload.workflow_run;
  const repository = `${context.repo.owner}/${context.repo.repo}`;
  if (
    run?.name !== "ci" ||
    run.conclusion !== "success" ||
    run.event !== "push" ||
    run.head_branch !== "main" ||
    run.head_repository?.full_name !== repository
  )
    throw new Error("Only successful main-branch push CI can start an automatic release.");
  return run;
}

async function automaticSource(github, context) {
  const run = trustedCompletion(context);
  const branch = await github.rest.repos.getBranch({ ...context.repo, branch: "main" });
  if (branch.data.commit.sha !== run.head_sha) return null;
  const config = await github.rest.repos.getContent({
    ...context.repo,
    path: ".github/release.json",
    ref: run.head_sha,
  });
  const { version } = JSON.parse(Buffer.from(config.data.content, "base64").toString("utf8"));
  return { sha: run.head_sha, tag: `v${parseReleaseTag(version)}` };
}

async function manualSource(github, context, tag) {
  const version = parseReleaseTag(tag);
  const name = `v${version}`;
  const commit = await github.rest.repos.getCommit({ ...context.repo, ref: name });
  const runs = await github.rest.actions.listWorkflowRuns({
    ...context.repo,
    workflow_id: "ci.yml",
    head_sha: commit.data.sha,
    event: "push",
    status: "success",
    per_page: 100,
  });
  const verified = runs.data.workflow_runs.some(
    (run) =>
      run.head_sha === commit.data.sha &&
      run.head_branch === "main" &&
      run.event === "push" &&
      run.conclusion === "success",
  );
  if (!verified)
    throw new Error("This tag has no successful main CI run. Fix CI before retrying release.");
  return { sha: commit.data.sha, tag: name };
}

/** Selects one verified commit. A version request persists across failed CI without polling. */
export async function planRelease({ github, context, tag, target = "all" }) {
  if (!["all", "desktop", "android"].includes(target)) throw new Error("Unknown release target.");
  const automatic = context.eventName === "workflow_run";
  if (!automatic && context.eventName !== "workflow_dispatch")
    throw new Error("Unsupported release event.");
  const source = automatic
    ? await automaticSource(github, context)
    : await manualSource(github, context, tag);
  if (!source) return { ready: false, reason: "A newer main commit superseded this run." };
  const release = await optional(() =>
    github.rest.repos.getReleaseByTag({ ...context.repo, tag: source.tag }),
  );
  if (automatic && release && !release.draft)
    return { ready: false, reason: "Requested version is already published." };
  const existing = await optional(() =>
    github.rest.repos.getCommit({ ...context.repo, ref: source.tag }),
  );
  if (existing && existing.sha !== source.sha)
    throw new Error(
      "Release tag belongs to another commit. Bump the requested version; never move a release tag.",
    );
  const newRelease = !release || release.draft;
  if (newRelease && target !== "all")
    throw new Error("A new release must build both desktop and Android.");
  return { ready: true, sha: source.sha, tag: source.tag, target, newRelease };
}
