import type { TailnetHost, TailnetStatus } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { pairingPayload, pairingQrText, parsePairingScan } from "./pairing";

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
});

const host = (over: Partial<TailnetHost> = {}): TailnetHost => ({
  found: true,
  state: "running",
  dnsName: "laptop.tail1.ts.net",
  ips: [],
  account: "",
  tailnet: "",
  servePort: 0,
  secureServePort: 0,
  takenPorts: [],
  reachable: false,
  phones: [],
  ...over,
});

const status = (
  over: Partial<TailnetStatus> = {},
  hostOver: Partial<TailnetHost> = {},
): TailnetStatus => ({
  enabled: false,
  state: "off",
  hostname: "",
  dnsName: "",
  ips: [],
  port: 47811,
  identity: "",
  loginUrl: "",
  funnel: false,
  error: "",
  host: host(hostOver),
  serverTime: "2026-10-03T10:00:00.000Z",
  ...over,
});

describe("the text of the QR code for a pairing", () => {
  it("is the daemon's own node's address on the daemon's port, with the code", () => {
    const online = status({ state: "online", dnsName: "marshal-dev.tail1.ts.net" });
    expect(pairingQrText(online, "7QX-2LD")).toBe(
      "marshal://pair?host=marshal-dev.tail1.ts.net%3A47811&code=7QX-2LD",
    );
  });

  it("is the tailnet port Tailscale Serve hands to the daemon, once this computer has reached it", () => {
    const served = status({}, { servePort: 47800, reachable: true });
    expect(pairingQrText(served, "7QX-2LD")).toBe(
      "marshal://pair?host=laptop.tail1.ts.net%3A47800&code=7QX-2LD",
    );
  });

  it("is null while the rule has not been seen to answer, while there is no address, and with no code", () => {
    expect(pairingQrText(status({}, { servePort: 47800, reachable: false }), "7QX-2LD")).toBeNull();
    expect(pairingQrText(status(), "7QX-2LD")).toBeNull();
    expect(pairingQrText(null, "7QX-2LD")).toBeNull();
    expect(pairingQrText(status({ state: "online", dnsName: "x.ts.net" }), undefined)).toBeNull();
  });
});
