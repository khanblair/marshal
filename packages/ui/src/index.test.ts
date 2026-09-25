import { buildIndex, isPublicModule } from "../scripts/index-source.mjs";
import * as ui from "./index";

const sources = import.meta.glob("./**/*.{ts,tsx}", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

describe("index.ts", () => {
  it("is up to date with src/. Run `pnpm gen` if this fails", () => {
    const modules = Object.entries(sources).map(([path, code]) => ({ path: path.slice(2), code }));
    expect(buildIndex(modules)).toBe(sources["./index.ts"]);
  });

  it("keeps tests, test setup, and underscore files private", () => {
    expect(isPublicModule("base/Button.tsx")).toBe(true);
    expect(isPublicModule("base/Button.test.tsx")).toBe(false);
    expect(isPublicModule("icons/_design-source.ts")).toBe(false);
    expect(isPublicModule("test-setup.ts")).toBe(false);
    expect(isPublicModule("index.ts")).toBe(false);
    expect(isPublicModule("styles.css")).toBe(false);
  });

  it("exports the components", () => {
    expect(typeof ui.Icon).toBe("function");
    expect(typeof ui.StatusDot).toBe("function");
  });
});
