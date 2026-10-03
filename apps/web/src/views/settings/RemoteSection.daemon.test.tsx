// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the person's sections are the daemon's).
import { chooser, daemon } from "~/testing/daemon-person-store";
import type { TailnetHost, TailnetStatus } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { wireProfile } from "~/testing/fake-me";
import { SettingsView } from "./SettingsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const made: string[] = [];

beforeEach(() => {
  made.length = 0;
  URL.createObjectURL = vi.fn(() => {
    made.push(`blob:test/${made.length + 1}`);
    return made.at(-1) as string;
  });
  URL.revokeObjectURL = vi.fn();
  // Every test starts from the golden person, the way the daemon was made.
  Object.assign(daemon.me.profile, wireProfile());
  daemon.me.avatar = null;
  daemon.emit("me", "me.updated", {
    profile: daemon.me.profile,
    preferences: daemon.me.preferences,
    progress: daemon.me.progress,
  });
  M.set({ settingsSection: "profile", toasts: [] });
});
afterEach(() => {
  cleanup();
  chooser.pick.mockReset();
  // A tailnet answer a test made up must not carry into the next test.
  vi.restoreAllMocks();
});

const toasts = () => M.S.toasts.map((toast) => toast.msg);

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

const tailnet = (
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

const open = () => {
  M.set({ settingsSection: "remote" });
  return render(() => <SettingsView />);
};

describe("the Remote control section on the daemon", () => {
  it("lists the daemon's paired devices, and asks before removing one through the daemon", async () => {
    open();
    expect(await screen.findByText("Pixel 8")).toBeInTheDocument();
    expect(screen.getByText("iPad Air")).toBeInTheDocument();
    fireEvent.click(screen.getAllByRole("button", { name: "Remove device" })[0] as HTMLElement);
    expect(M.S.dialog).toMatchObject({
      title: "Remove device",
      message: "Pixel 8 will lose access to Marshal and must pair again to reconnect.",
      destructive: true,
    });
    M.S.dialog?.run();
    await waitFor(() => expect(toasts()).toContain("Device removed"));
    expect(daemon.routes()).toContain("DELETE /v1/me/devices/01H1234567890ABCDEFGHJKMNPQ");
  });

  it("shows the code the daemon made after Pair a device", async () => {
    open();
    expect(screen.queryByText("7QX-2LD")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Pair a device" }));
    expect(await screen.findByText("7QX-2LD")).toBeInTheDocument();
    expect(screen.getByText("Enter this code on the device")).toBeInTheDocument();
    expect(screen.getByText("7QX-2LD").tagName).toBe("CODE");
  });

  it("says Tailscale is not on this computer, and shows no made-up account or machine, when it was not found", async () => {
    open();
    expect(await screen.findByText("Tailscale is not on this computer")).toBeInTheDocument();
    expect(screen.queryByText("ada@kolaborate.co")).toBeNull();
    expect(screen.queryByText("marshal-laptop.tail3f2a.ts.net")).toBeNull();
    expect(screen.getByText("Not signed in")).toBeInTheDocument();
    expect(screen.getByText("Not on a tailnet")).toBeInTheDocument();
    expect(screen.getByText("Not installed")).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "Pairing QR code" })).toBeNull();
  });

  it("names this computer, gives the exact command with the real port, and offers a copy of it, when Tailscale hands nothing to Marshal", async () => {
    vi.spyOn(M, "tailnetStatus").mockResolvedValue(tailnet());
    const write = vi.fn(async () => undefined);
    Object.assign(navigator, { clipboard: { writeText: write } });
    open();
    const command = "tailscale serve --bg --http=47800 http://127.0.0.1:47801";
    expect(await screen.findByText(command)).toBeInTheDocument();
    expect(
      screen.getByText(/This computer is laptop\.tail1234\.ts\.net on your tailnet/),
    ).toBeInTheDocument();
    expect(screen.getByText("owner@example.com")).toBeInTheDocument();
    fireEvent.click(screen.getAllByRole("button", { name: "Copy" })[0] as HTMLElement);
    await waitFor(() => expect(write).toHaveBeenCalledWith(command));
    expect(toasts()).toContain("Command copied");
  });

  it("asks again when Check again is pressed, and shows the address once Serve hands the port to Marshal", async () => {
    const asked = vi.spyOn(M, "tailnetStatus").mockResolvedValue(tailnet());
    open();
    await screen.findByText(/nothing hands a port to Marshal/);
    asked.mockResolvedValue(tailnet({}, { servePort: 47800, reachable: true }));
    fireEvent.click(screen.getByRole("button", { name: "Check again" }));
    expect(await screen.findByText("laptop.tail1234.ts.net:47800")).toBeInTheDocument();
    expect(screen.getByText(/this computer reached it there/)).toBeInTheDocument();
    expect(screen.queryByText(/nothing hands a port to Marshal/)).toBeNull();
  });

  it("adds a QR code once there is an address that answers", async () => {
    vi.spyOn(M, "tailnetStatus").mockResolvedValue(
      tailnet({}, { servePort: 47800, reachable: true }),
    );
    open();
    await screen.findByText("laptop.tail1234.ts.net:47800");
    fireEvent.click(screen.getByRole("button", { name: "Pair a device" }));
    expect(await screen.findByRole("img", { name: "Pairing QR code" })).toBeInTheDocument();
    expect(screen.getByText(/choose Scan the QR code/)).toBeInTheDocument();
  });

  it("explains why there is no QR code while the address does not work", async () => {
    open();
    await screen.findByText("Tailscale is not on this computer");
    fireEvent.click(screen.getByRole("button", { name: "Pair a device" }));
    await screen.findByText("7QX-2LD");
    expect(
      await screen.findByText(/A QR code appears once the phone address above works/),
    ).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "Pairing QR code" })).toBeNull();
  });

  it("uses Marshal's own node's address when it is online", async () => {
    vi.spyOn(M, "tailnetStatus").mockResolvedValue(
      tailnet({
        enabled: true,
        state: "online",
        dnsName: "marshal-dev.tail1234.ts.net",
        identity: "owner@example.com",
      }),
    );
    open();
    expect(await screen.findByText("marshal-dev.tail1234.ts.net:47801")).toBeInTheDocument();
    expect(screen.getByText("On your tailnet")).toBeInTheDocument();
  });

  it("lists the phones on the tailnet, and says when one is offline and since when", async () => {
    vi.spyOn(M, "tailnetStatus").mockResolvedValue(
      tailnet(
        {},
        {
          phones: [
            { name: "Pixel", os: "android", online: false, lastSeen: "2026-09-24T10:00:00.000Z" },
            { name: "iPhone", os: "iOS", online: true, lastSeen: "2026-10-03T10:00:00.000Z" },
          ],
        },
      ),
    );
    open();
    expect(await screen.findByText("Online on your tailnet")).toBeInTheDocument();
    expect(screen.getByText("iPhone")).toBeInTheDocument();
    expect(screen.getByText(/Offline, last seen/)).toBeInTheDocument();
  });

  it("says no phone is signed in to the tailnet yet, naming the account to sign in as", async () => {
    vi.spyOn(M, "tailnetStatus").mockResolvedValue(tailnet());
    open();
    expect(
      await screen.findByText(/No phone or tablet is signed in to this tailnet yet/),
    ).toHaveTextContent("owner@example.com");
  });
});
