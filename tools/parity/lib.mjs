/**
 * Parity harness helpers: serve the Claude Design export, load either app
 * deterministically, capture screenshots, and diff them.
 */
import { createReadStream, existsSync, statSync } from "node:fs";
import { createServer } from "node:http";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "@playwright/test";
import pixelmatch from "pixelmatch";
import { PNG } from "pngjs";

export const repoRoot = join(fileURLToPath(new URL(".", import.meta.url)), "..", "..");
export const designDir = join(repoRoot, "design");
export const outDir = join(repoRoot, ".parity");

/** Fixed wall clock so relative times like "4 min ago" never change between runs. */
export const FIXED_TIME = new Date("2026-09-24T10:00:00");

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json",
  ".css": "text/css",
  ".png": "image/png",
  ".jpg": "image/jpeg",
};

/** Serves the design export folder over HTTP (the prototype imports sibling files by fetch). */
export function serveDesign(port = 4399) {
  const server = createServer((req, res) => {
    const url = decodeURIComponent((req.url ?? "/").split("?")[0] ?? "/");
    const file = normalize(join(designDir, url === "/" ? "Marshal.dc.html" : url));
    if (!file.startsWith(designDir) || !existsSync(file) || !statSync(file).isFile()) {
      res.writeHead(404).end("not found");
      return;
    }
    res.writeHead(200, { "content-type": MIME[extname(file)] ?? "application/octet-stream" });
    createReadStream(file).pipe(res);
  });
  return new Promise((resolve) => server.listen(port, () => resolve(server)));
}

export const SIZES = {
  phone: { width: 390, height: 844 },
  tablet: { width: 820, height: 1180 },
  desktop: { width: 1440, height: 900 },
};

/**
 * The prototype draws its own device and reset toolbar over the app. The port does not have
 * one (removed on request), so it is hidden in the prototype before screenshots and text capture.
 */
/**
 * The prototype's template engine wraps every interpolation in an HTML span, and an HTML span
 * inside an SVG text element never renders, so the prototype's charts have no axis labels. The
 * port draws them, as the design's markup asks. Both apps hide chart text, so the rest compares.
 */
const CHART_TEXT_HIDDEN = 'svg[role="img"] text{display:none!important}';

const PROTO_TOOLBAR_HIDDEN =
  '[role="toolbar"][aria-label="Prototype controls"]{display:none!important}';

/**
 * Opens one app in a fresh page with a frozen clock, no simulation (#nosim),
 * onboarding skipped, and the requested size and color scheme. `hideProtoToolbar` is for the
 * prototype only.
 */
export async function openApp(
  browser,
  { url, size, theme = "light", onboarded = true, hash = "nosim", hideProtoToolbar = false },
) {
  const context = await browser.newContext({
    viewport: SIZES[size] ?? size,
    colorScheme: "light",
    deviceScaleFactor: 1,
    reducedMotion: "no-preference",
  });
  const page = await context.newPage();
  await page.clock.install({ time: FIXED_TIME });
  await page.addInitScript(
    ([flag]) => {
      try {
        localStorage.setItem("marshal-proto-device", "fit");
        if (flag) localStorage.setItem("marshal-proto-onboarded", "1");
        else localStorage.removeItem("marshal-proto-onboarded");
      } catch {
        /* storage can be blocked */
      }
    },
    [onboarded],
  );
  await page.goto(`${url}#${hash}`, { waitUntil: "load" });
  await page.waitForFunction(() => Boolean(window.M?.S?.ready), null, { timeout: 30000 });
  await page.evaluate(() => document.fonts.ready);
  await page.addStyleTag({ content: CHART_TEXT_HIDDEN });
  if (hideProtoToolbar) await page.addStyleTag({ content: PROTO_TOOLBAR_HIDDEN });
  await page.clock.pauseAt(new Date(FIXED_TIME.getTime() + 5000));
  await settle(page);
  if (theme === "dark") {
    await page.evaluate(() => window.M.setTheme("dark"));
    await settle(page);
  }
  return { context, page };
}

/** Lets rAF and timers flush while the clock is paused. */
export async function settle(page, ms = 160) {
  await page.clock.runFor(ms);
  await page.waitForTimeout(60);
  await page.clock.runFor(32);
}

export async function launch() {
  return chromium.launch({ args: ["--font-render-hinting=none", "--disable-lcd-text"] });
}

/** Normalized visible text, used as a second parity signal next to pixels. */
export async function pageText(page) {
  return page.evaluate(() =>
    document.body.innerText
      .split("\n")
      .map((s) => s.replace(/\s+/g, " ").trim())
      .filter(Boolean),
  );
}

export function diffImages(a, b, { threshold = 0.1 } = {}) {
  const A = PNG.sync.read(a);
  const B = PNG.sync.read(b);
  const width = Math.max(A.width, B.width);
  const height = Math.max(A.height, B.height);
  const pad = (img) => {
    if (img.width === width && img.height === height) return img;
    const out = new PNG({ width, height, fill: true });
    PNG.bitblt(img, out, 0, 0, img.width, img.height, 0, 0);
    return out;
  };
  const PA = pad(A);
  const PB = pad(B);
  const diff = new PNG({ width, height });
  const count = pixelmatch(PA.data, PB.data, diff.data, width, height, { threshold });
  return {
    count,
    total: width * height,
    percent: (count / (width * height)) * 100,
    sizeMismatch: A.width !== B.width || A.height !== B.height,
    png: PNG.sync.write(diff),
  };
}
