// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the alert settings are the daemon's).
import { daemon } from "~/testing/daemon-person-store";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { golden } from "~/data/testing/golden";
import type { AlertSettings } from "@marshal/protocol";
import { M } from "~/mock";
import { SettingsView } from "./SettingsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * Section S26c: where each alert goes. The daemon's router holds the choices and follows a change at
 * once, so the screen mirrors what it says: one checkbox per alert and channel, and a change is one
 * PUT that answers the whole settings.
 */

const PUT = "PUT /v1/settings/alerts";
const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);
const row = (label: string): HTMLElement => screen.getByRole("group", { name: label });

beforeEach(async () => {
  daemon.alerts.settings = golden<AlertSettings>("alert-settings");
  await M.reconnect();
  await waitFor(() => expect(M.alerts()?.routes).toHaveLength(3));
  M.set({ settingsSection: "alerts", toasts: [] });
});
afterEach(cleanup);

describe("the Alerts section on the daemon", () => {
  it("shows each alert with the channels it goes to, and says which channel is not connected", () => {
    render(() => <SettingsView />);
    const stuck = row("An agent is stuck or needs you");
    expect(within(stuck).getByLabelText("Telegram")).toBeChecked();
    expect(within(stuck).getByLabelText("Discord", { exact: false })).not.toBeChecked();
    expect(within(stuck).getByLabelText("ntfy")).not.toBeChecked();
    expect(within(stuck).getByText("Not connected", { selector: "span" })).toBeInTheDocument();
    expect(within(row("CI failed")).getByLabelText("Telegram")).not.toBeChecked();
  });

  it("sends only the alert that changed, draws it at once, and says it was saved", async () => {
    render(() => <SettingsView />);
    const ci = row("CI failed");
    fireEvent.click(within(ci).getByLabelText("ntfy"));
    expect(within(ci).getByLabelText("ntfy")).toBeChecked();
    await waitFor(() => expect(toasts()).toContain("Saved"));
    expect(daemon.bodies(PUT).at(-1)).toEqual({
      routes: [{ event: "ci.failed", channels: ["ntfy"] }],
    });
    expect(daemon.alerts.settings.routes.find((r) => r.event === "ci.failed")?.channels).toEqual([
      "ntfy",
    ]);
  });

  it("takes a channel away, and puts the box back with the daemon's sentence when it refuses", async () => {
    render(() => <SettingsView />);
    const stuck = row("An agent is stuck or needs you");
    daemon.refuseNext(PUT, 400, "invalid_argument", "Choose Telegram, Discord, or ntfy.");
    fireEvent.click(within(stuck).getByLabelText("Telegram"));
    await waitFor(() => expect(toasts()).toContain("Choose Telegram, Discord, or ntfy."));
    expect(within(stuck).getByLabelText("Telegram")).toBeChecked();
  });

  it("turns on staying quiet during calendar events, draws it at once, and keeps it", async () => {
    render(() => <SettingsView />);
    const box = screen.getByLabelText(/Stay quiet during calendar events/);
    expect(box).not.toBeChecked();
    fireEvent.click(box);
    expect(box).toBeChecked();
    await waitFor(() => expect(toasts()).toContain("Saved"));
    expect(daemon.bodies(PUT).at(-1)).toEqual({ routes: [], quietDuringEvents: true });
    expect(daemon.alerts.settings.quietDuringEvents).toBe(true);
    expect(screen.getByText(/Approvals always come through at once/)).toBeInTheDocument();
  });
});
