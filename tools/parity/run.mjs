/**
 * Parity runner: renders the original Claude Design prototype and the SolidJS
 * port with the same frozen clock, size, theme, and steps, then compares the
 * screenshots pixel by pixel and the visible text line by line.
 *
 *   pnpm parity                       run every scenario
 *   pnpm parity --only board          scenarios whose id contains "board"
 *   pnpm parity --size phone          one size (phone, tablet, desktop)
 *   pnpm parity --theme dark          one theme
 *   pnpm parity --ids a,b,c           exactly these scenario ids
 *   pnpm parity --app http://...      use a running dev server instead of starting one
 *   pnpm parity --tolerance 0.2       allowed percent of differing pixels (default 0.15)
 *   pnpm parity --ref-only            render only the prototype (to check a scenario's steps)
 *   pnpm parity --design-port 4400    port for the prototype server (use one per parallel run)
 *   pnpm parity --app-port 5301       port for the Vite server the runner starts (default 5199)
 *   pnpm parity --shard 1/4           run every 4th scenario, starting with the second
 *   pnpm parity --from 120            skip the first 120 scenarios
 *
 * The prototype's own floating toolbar is hidden on the prototype side; the port has none.
 * Output: .parity/<scenario>/<size>-<theme>/{ref,app,diff}.png and report.json
 */
import { spawn } from "node:child_process";
import { mkdirSync, readdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import {
  diffImages,
  launch,
  openApp,
  outDir,
  pageText,
  repoRoot,
  serveDesign,
  settle,
} from "./lib.mjs";

const args = process.argv.slice(2);
const flag = (name) => {
  const i = args.indexOf(`--${name}`);
  return i >= 0 ? args[i + 1] : undefined;
};
const only = flag("only");
const onlyIds = flag("ids")?.split(",");
const onlySize = flag("size");
const onlyTheme = flag("theme");
const tolerance = Number(flag("tolerance") ?? 0.15);
const refOnly = args.includes("--ref-only");
const DESIGN_PORT = Number(flag("design-port") ?? 4399);
const APP_PORT = Number(flag("app-port") ?? 5199);
const SERVER_WAIT_MS = 60000;
const POLL_MS = 300;
const MAX_FAILURES_SHOWN = 12;
const MAX_LINES_SHOWN = 4;
const CLOSED_PATTERN = /has been closed|Target crashed/;
const CLOCK_PATTERN = /Cannot fast-forward to the past/;
const MAX_ATTEMPTS = 3;
const [SHARD_INDEX, SHARD_COUNT] = (flag("shard") ?? "0/1").split("/").map(Number);
const FIRST_INDEX = Number(flag("from") ?? 0);

async function loadScenarios() {
  const dir = join(repoRoot, "tools", "parity", "scenarios");
  const list = [];
  for (const file of readdirSync(dir)
    .filter((f) => f.endsWith(".mjs"))
    .sort()) {
    const mod = await import(pathToFileURL(join(dir, file)).href);
    list.push(...mod.default);
  }
  return list
    .filter((s) => !only || s.id.includes(only))
    .filter((s) => !onlyIds || onlyIds.includes(s.id))
    .filter((_, i) => i >= FIRST_INDEX && (i - FIRST_INDEX) % SHARD_COUNT === SHARD_INDEX);
}

async function waitForUrl(url) {
  const deadline = Date.now() + SERVER_WAIT_MS;
  while (Date.now() < deadline) {
    try {
      if ((await fetch(url)).ok) return;
    } catch {
      /* not up yet */
    }
    await new Promise((r) => setTimeout(r, POLL_MS));
  }
  throw new Error(`Timed out waiting for ${url}`);
}

function startApp() {
  return spawn(
    "pnpm",
    ["--filter", "web", "exec", "vite", "--port", String(APP_PORT), "--strictPort"],
    { cwd: repoRoot, stdio: "ignore" },
  );
}

/** Lines in `a` that are missing from `b`, counting repeats. */
function missing(a, b) {
  const counts = new Map();
  for (const line of b) counts.set(line, (counts.get(line) ?? 0) + 1);
  const out = [];
  for (const line of a) {
    const n = counts.get(line) ?? 0;
    if (n > 0) counts.set(line, n - 1);
    else out.push(line);
  }
  return out;
}

async function capture(browser, url, scenario, { size, theme, isPrototype }) {
  const { context, page } = await openApp(browser, {
    url,
    size,
    theme,
    onboarded: scenario.onboarded ?? true,
    hash: scenario.hash ?? "nosim",
    hideProtoToolbar: isPrototype,
  });
  try {
    if (scenario.steps) await scenario.steps(page, { size, theme });
    await settle(page);
    const shot = await page.screenshot({ animations: "disabled", caret: "hide" });
    return { shot, text: await pageText(page) };
  } finally {
    await context.close();
  }
}

const designUrl = () => `http://localhost:${DESIGN_PORT}/Marshal.dc.html`;

function writeImages(scenario, variant, images) {
  const dir = join(outDir, scenario.id, `${variant.size}-${variant.theme}`);
  mkdirSync(dir, { recursive: true });
  for (const [name, data] of Object.entries(images)) writeFileSync(join(dir, name), data);
  return dir;
}

async function runOne(browser, appUrl, scenario, variant) {
  const { size, theme } = variant;
  const ref = await capture(browser, designUrl(), scenario, { ...variant, isPrototype: true });
  if (refOnly) {
    writeImages(scenario, variant, { "ref.png": ref.shot });
    return { id: scenario.id, size, theme, ok: true, percent: 0 };
  }
  const app = await capture(browser, appUrl, scenario, { ...variant, isPrototype: false });
  const diff = diffImages(ref.shot, app.shot);
  const dir = writeImages(scenario, variant, {
    "ref.png": ref.shot,
    "app.png": app.shot,
    "diff.png": diff.png,
  });
  const missingInApp = missing(ref.text, app.text);
  const extraInApp = missing(app.text, ref.text);
  const limit = scenario.tolerance ?? tolerance;
  const ok =
    diff.percent <= limit && !diff.sizeMismatch && !missingInApp.length && !extraInApp.length;
  const result = {
    id: scenario.id,
    size,
    theme,
    ok,
    percent: diff.percent,
    sizeMismatch: diff.sizeMismatch,
    missingInApp,
    extraInApp,
  };
  writeFileSync(join(dir, "report.json"), JSON.stringify(result, null, 2));
  return result;
}

/**
 * Runs one variant. If the browser died, starts a new one. If the machine was so busy that the
 * page outran the frozen clock, tries again. Gives up after a few attempts.
 */
async function runGuarded(session, appUrl, scenario, variant) {
  for (let attempt = 1; ; attempt++) {
    try {
      return await runOne(session.browser, appUrl, scenario, variant);
    } catch (error) {
      const message = String(error.message ?? error);
      const retryable = CLOSED_PATTERN.test(message) || CLOCK_PATTERN.test(message);
      if (!retryable || attempt >= MAX_ATTEMPTS) throw error;
      if (CLOSED_PATTERN.test(message)) session.browser = await launch();
    }
  }
}

function describe(r) {
  const flagText = r.ok ? "pass" : "FAIL";
  const detail = r.error
    ? ` error: ${r.error.split("\n")[0]}`
    : ` text -${r.missingInApp?.length ?? 0} +${r.extraInApp?.length ?? 0}`;
  return `${flagText}  ${r.id.padEnd(34)} ${r.size.padEnd(8)} ${r.theme.padEnd(5)} ${r.percent.toFixed(3)}%${detail}`;
}

function* variantsOf(scenario) {
  for (const size of scenario.sizes ?? ["desktop", "tablet", "phone"]) {
    if (onlySize && size !== onlySize) continue;
    for (const theme of scenario.themes ?? ["light", "dark"]) {
      if (onlyTheme && theme !== onlyTheme) continue;
      yield { size, theme };
    }
  }
}

function printFailures(failed) {
  for (const r of failed.slice(0, MAX_FAILURES_SHOWN)) {
    for (const line of (r.missingInApp ?? []).slice(0, MAX_LINES_SHOWN))
      console.log(`  ${r.id} ${r.size}: missing in app: ${line}`);
    for (const line of (r.extraInApp ?? []).slice(0, MAX_LINES_SHOWN))
      console.log(`  ${r.id} ${r.size}: extra in app: ${line}`);
  }
}

async function runAll(session, appUrl, scenarios) {
  const results = [];
  for (const scenario of scenarios) {
    for (const variant of variantsOf(scenario)) {
      const r = await runGuarded(session, appUrl, scenario, variant).catch((error) => ({
        id: scenario.id,
        ...variant,
        ok: false,
        percent: 100,
        error: String(error.message ?? error),
      }));
      results.push(r);
      console.log(describe(r));
    }
  }
  return results;
}

async function main() {
  const scenarios = await loadScenarios();
  if (!scenarios.length) {
    console.log("No scenarios match.");
    return 0;
  }
  const designServer = await serveDesign(DESIGN_PORT);
  const appUrl = flag("app") ?? `http://localhost:${APP_PORT}/`;
  const child = flag("app") || refOnly ? null : startApp();
  const session = { browser: await launch() };
  let results = [];
  try {
    if (!refOnly) await waitForUrl(appUrl);
    results = await runAll(session, appUrl, scenarios);
  } finally {
    await session.browser.close().catch(() => {});
    designServer.close();
    child?.kill();
  }
  const failed = results.filter((r) => !r.ok);
  console.log(`\n${results.length - failed.length}/${results.length} passed. Reports in .parity/`);
  printFailures(failed);
  return failed.length ? 1 : 0;
}

process.exit(await main());
