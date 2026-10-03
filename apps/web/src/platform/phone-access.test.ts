import type { TailnetHost, TailnetStatus } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { phoneAccess, serveCommand } from "./phone-access";

const host = (over: Partial<TailnetHost> = {}): TailnetHost => ({
  found: true,
  state: "running",
  dnsName: "laptop.tail1234.ts.net",
  ips: ["100.64.0.1"],
  account: "owner@example.com",
  tailnet: "owner@example.com",
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
  port: 47801,
  identity: "",
  loginUrl: "",
  funnel: false,
  error: "",
  host: host(hostOver),
  serverTime: "2026-10-03T10:00:00.000Z",
  ...over,
});

describe("how a phone reaches this computer", () => {
  it("knows nothing while the status is not known", () => {
    expect(phoneAccess(null)).toBeNull();
    expect(phoneAccess(undefined)).toBeNull();
  });

  it("uses the daemon's own node when it is online, with the daemon's port", () => {
    const access = phoneAccess(
      status({ enabled: true, state: "online", dnsName: "marshal-dev.tail1234.ts.net" }),
    );
    expect(access).toMatchObject({
      address: "marshal-dev.tail1234.ts.net:47801",
      via: "node",
      next: null,
    });
  });

  it("uses Tailscale on this computer when Serve hands a plain port to the daemon, and says it was reached", () => {
    const access = phoneAccess(status({}, { servePort: 47800, reachable: true }));
    expect(access).toMatchObject({
      address: "laptop.tail1234.ts.net:47800",
      via: "serve",
      next: null,
    });
    expect(access?.checked).toContain("reached it there");
  });

  it("gives the address but says it did not answer when the rule is there and the check failed", () => {
    const access = phoneAccess(status({}, { servePort: 47800, reachable: false }));
    expect(access?.address).toBe("laptop.tail1234.ts.net:47800");
    expect(access?.checked).toContain("did not answer");
    expect(access?.next?.command).toBe("tailscale serve status");
  });

  it("names the computer and gives the exact command when Tailscale runs and hands nothing to Marshal", () => {
    const access = phoneAccess(status());
    expect(access?.address).toBeNull();
    expect(access?.next?.detail).toContain("laptop.tail1234.ts.net");
    expect(access?.next?.command).toBe("tailscale serve --bg --http=47800 http://127.0.0.1:47801");
    expect(access?.next?.alternative).toContain("--tailnet");
  });

  it("suggests the next free tailnet port, and says so, when 47800 is already used", () => {
    const access = phoneAccess(status({}, { takenPorts: [47800, 47801] }));
    expect(access?.next?.command).toBe("tailscale serve --bg --http=47802 http://127.0.0.1:47801");
    expect(access?.next?.detail).toContain("Tailnet port 47800 is already used");
    expect(access?.next?.detail).toContain("laptop.tail1234.ts.net:47802");
  });

  it("suggests port 47801 when only 47800 is used, even though the daemon is on 47801 too", () => {
    const access = phoneAccess(status({}, { takenPorts: [47800] }));
    expect(access?.next?.command).toBe("tailscale serve --bg --http=47801 http://127.0.0.1:47801");
  });

  it("says an https-only rule is no use to the phone app, and asks for a plain http one", () => {
    const access = phoneAccess(status({}, { secureServePort: 443 }));
    expect(access?.next?.title).toContain("https only");
    expect(access?.next?.detail).toContain("port 443");
    expect(access?.next?.command).toContain("--http=47800");
  });

  it.each([
    ["not-running", "app is not running"],
    ["stopped", "turned off"],
    ["needs-login", "sign in on this computer"],
    ["needs-approval", "admin console"],
    ["starting", "starting"],
  ])("explains a Tailscale that is %s", (state, words) => {
    const access = phoneAccess(status({}, { state }));
    expect(access?.address).toBeNull();
    expect(access?.next?.detail).toContain(words);
  });

  it("says Tailscale is not on this computer when it was not found", () => {
    const access = phoneAccess(status({}, { found: false, state: "", dnsName: "" }));
    expect(access?.next?.title).toBe("Tailscale is not on this computer");
    expect(access?.checked).toContain("did not find it");
  });

  it("carries the sign-in address while the daemon's own node waits, and the reason when it failed", () => {
    const waiting = phoneAccess(
      status({ enabled: true, state: "signing-in", loginUrl: "https://login.example/abc" }),
    );
    expect(waiting?.next?.link?.href).toBe("https://login.example/abc");
    const failed = phoneAccess(status({ enabled: true, state: "error", error: "no network" }));
    expect(failed?.next?.detail).toBe("no network");
  });

  it("builds the serve command from the two ports", () => {
    expect(serveCommand(47800, 47811)).toBe(
      "tailscale serve --bg --http=47800 http://127.0.0.1:47811",
    );
  });
});
