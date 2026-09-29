import { describe, expect, it } from "vitest";
import { pairingPayload, parsePairingScan, tailnetHost } from "./pairing";

describe("the pairing QR code", () => {
  it("carries the address and the code, and reads back as the code", () => {
    const payload = pairingPayload("laptop.tail1.ts.net:47800", "7QX-2LD");
    expect(payload).toBe("marshal://pair?host=laptop.tail1.ts.net%3A47800&code=7QX-2LD");
    expect(parsePairingScan(payload)).toBe("7QX-2LD");
  });

  it("reads a bare code, and nothing that is not a code", () => {
    expect(parsePairingScan(" 7qx-2ld ")).toBe("7qx-2ld");
    for (const other of [
      "https://example.com",
      "marshal://card/api",
      "marshal://pair?host=x",
      "hello world",
      "",
      null,
    ]) {
      expect(parsePairingScan(other)).toBeNull();
    }
  });

  it("uses the page's port, or the daemon's usual one when the page has none", () => {
    expect(tailnetHost("laptop.tail1.ts.net", "5000")).toBe("laptop.tail1.ts.net:5000");
    expect(tailnetHost("laptop.tail1.ts.net", "")).toBe("laptop.tail1.ts.net:47800");
  });
});
