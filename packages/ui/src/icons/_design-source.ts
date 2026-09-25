/**
 * Test helper: the prototype's source files, read as text at build time, so
 * tests can check the port against the design. Not part of the public API.
 */
const files = import.meta.glob(
  [
    "../../../../design/*.dc.html",
    "!../../../../design/Marshal Showcase.dc.html",
    "../../../../design/store.js",
  ],
  { query: "?raw", import: "default", eager: true },
) as Record<string, string>;

/** File name to file text, for every design file except the marketing posters. */
export const designFiles: Record<string, string> = Object.fromEntries(
  Object.entries(files).map(([path, text]) => [path.split("/").pop() ?? path, text]),
);

/** Text of design/store.js. */
export const storeSource: string = designFiles["store.js"] ?? "";
