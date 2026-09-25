/** Smoke test: renders the original prototype headlessly and saves a screenshot. */
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { launch, openApp, outDir, pageText, serveDesign } from "./lib.mjs";

const server = await serveDesign();
const browser = await launch();
try {
  mkdirSync(outDir, { recursive: true });
  for (const [size, theme] of [
    ["desktop", "light"],
    ["phone", "dark"],
  ]) {
    const { context, page } = await openApp(browser, {
      url: "http://localhost:4399/Marshal.dc.html",
      size,
      theme,
    });
    const shot = await page.screenshot({ animations: "disabled" });
    writeFileSync(join(outDir, `smoke-ref-${size}-${theme}.png`), shot);
    const text = await pageText(page);
    console.log(`${size}/${theme}: ${text.length} text lines, first: ${JSON.stringify(text.slice(0, 6))}`);
    await context.close();
  }
} finally {
  await browser.close();
  server.close();
}
