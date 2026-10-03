import { describe, expect, it } from "vitest";
import { pairingPayload, pairingQrText, parsePairingScan, tailnetHost } from "./pairing";

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

  it("uses the port it is given, or the daemon's usual one when there is none", () => {
    expect(tailnetHost("laptop.tail1.ts.net", "5000")).toBe("laptop.tail1.ts.net:5000");
    expect(tailnetHost("laptop.tail1.ts.net", 47811)).toBe("laptop.tail1.ts.net:47811");
    expect(tailnetHost("laptop.tail1.ts.net", "")).toBe("laptop.tail1.ts.net:47800");
  });
});

describe("the text of the QR code for a pairing", () => {
  const online = { state: "online", dnsName: "laptop.tail1.ts.net", port: 47811 };

  it("is the node's address on the daemon's port, with the code", () => {
    expect(pairingQrText(online, "7QX-2LD")).toBe(
      "marshal://pair?host=laptop.tail1.ts.net%3A47811&code=7QX-2LD",
    );
  });

  it("falls back to the page's port for a daemon that does not say", () => {
    const pagePort = window.location.port || "47800";
    expect(pairingQrText({ ...online, port: 0 }, "7QX-2LD")).toContain(`%3A${pagePort}`);
  });

  it("is null until a phone can reach this computer, and until there is a code", () => {
    expect(pairingQrText({ ...online, state: "signing-in" }, "7QX-2LD")).toBeNull();
    expect(pairingQrText({ ...online, state: "off", dnsName: "" }, "7QX-2LD")).toBeNull();
    expect(pairingQrText(null, "7QX-2LD")).toBeNull();
    expect(pairingQrText(online, undefined)).toBeNull();
  });
});
