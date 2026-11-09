import { describe, expect, it } from "vitest";
import { isLoopbackHttpUrl } from "./preview-link";

describe("loopback preview links", () => {
  it("recognizes localhost HTTP URLs", () => {
    expect(isLoopbackHttpUrl("http://localhost:3000")).toBe(true);
    expect(isLoopbackHttpUrl("http://127.0.0.1:5173/path")).toBe(true);
    expect(isLoopbackHttpUrl("http://[::1]:8080")).toBe(true);
  });

  it("does not intercept public hosts or non-HTTP schemes", () => {
    expect(isLoopbackHttpUrl("https://localhost:3000")).toBe(false);
    expect(isLoopbackHttpUrl("http://example.com")).toBe(false);
    expect(isLoopbackHttpUrl("javascript:alert(1)")).toBe(false);
    expect(isLoopbackHttpUrl("not a URL")).toBe(false);
  });
});
