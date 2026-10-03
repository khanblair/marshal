import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { buildThemeCss, currentThemeCss } from "./build-splash-theme.mjs";

const splash = join(dirname(fileURLToPath(import.meta.url)), "..", "splash");

test("the first screen's theme is the design tokens as they are now", () => {
  assert.equal(
    readFileSync(join(splash, "theme.css"), "utf8"),
    currentThemeCss(),
    "splash/theme.css has drifted from packages/tokens. Run `pnpm --filter mobile theme`.",
  );
});

test("light is the default and the phone's dark setting switches to the dark colors", () => {
  const css = buildThemeCss(
    "@theme {\n  --color-canvas: #FFFFFF;\n  --color-ink: #111111;\n}\n",
    '[data-theme="dark"] {\n  --color-canvas: #000000;\n  --color-ink: #EEEEEE;\n}\n',
  );
  assert.match(css, /:root \{[^}]*--color-canvas: #FFFFFF;/s);
  assert.match(
    css,
    /@media \(prefers-color-scheme: dark\) \{\s*:root \{[^}]*--color-canvas: #000000;/s,
  );
});

test("a token file with no colors is refused rather than written as an empty theme", () => {
  assert.throws(() => buildThemeCss("@theme {\n}\n", '[data-theme="dark"] {\n}\n'));
  assert.throws(() => buildThemeCss("nothing", "nothing"));
});

test("every font and image the screen asks for is a file in the app", () => {
  const css = readFileSync(join(splash, "theme.css"), "utf8");
  const html = readFileSync(join(splash, "index.html"), "utf8");
  for (const [, url] of css.matchAll(/url\("([^"]+)"\)/g))
    assert.ok(existsSync(join(splash, url)), url);
  for (const [, src] of html.matchAll(/(?:src|href)="\.\/([^"]+)"/g))
    assert.ok(existsSync(join(splash, src)), src);
});
