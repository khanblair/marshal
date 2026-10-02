import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Integration } from "~/mock";
import { createEditState } from "./edit-state";
import { GoogleCalendarForm } from "./GoogleCalendarForm";

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const row = (st: Integration["st"]): Integration => ({
  id: "gcal",
  name: "Google Calendar",
  icon: "calendar",
  st,
  detail: "",
});

const ONE_CLICK = async () => ({ bundled: true, own: false });
const NO_CLIENT = async () => ({ bundled: false, own: false });

function show(
  st: Integration["st"],
  extra: Partial<Parameters<typeof GoogleCalendarForm>[0]> = {},
) {
  const edit = createEditState();
  edit.open("gcal");
  render(() => (
    <GoogleCalendarForm
      integration={row(st)}
      edit={edit}
      info={ONE_CLICK}
      refresh={async () => undefined}
      {...extra}
    />
  ));
  return edit;
}

describe("Connect with Google", () => {
  it("is one button and pastes nothing when Marshal's own Google client is built in", async () => {
    show("none");
    await screen.findByRole("button", { name: "Connect with Google" });
    expect(screen.getByLabelText("OAuth client id")).not.toBeVisible();
    expect(screen.getByText(/It can only read, never change anything/)).toBeInTheDocument();
  });

  it("hides the paste form under 'Use my own Google client instead'", async () => {
    show("none");
    await screen.findByRole("button", { name: "Connect with Google" });
    const summary = screen.getByText("Use my own Google client instead");
    expect(summary.closest("details")).not.toHaveAttribute("open");
  });

  it("opens Google on one click and then asks the row how it went until the grant lands", async () => {
    const grant = vi.fn(async () => true);
    const refresh = vi.fn(async () => undefined);
    show("none", { grant, refresh });
    fireEvent.click(await screen.findByRole("button", { name: "Connect with Google" }));
    await waitFor(() => expect(grant).toHaveBeenCalledTimes(1));
    await vi.advanceTimersByTimeAsync(4100);
    expect(refresh.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it("says so when Google could not be started, and stops asking", async () => {
    const edit = show("none", { grant: async () => false });
    fireEvent.click(await screen.findByRole("button", { name: "Connect with Google" }));
    await waitFor(() =>
      expect(edit.errorFor("gcal")).toMatch(/could not start the Google sign-in/),
    );
  });

  it("offers Reconnect, Test and Disconnect once connected, and no Connect button", async () => {
    show("connected");
    await screen.findByRole("button", { name: "Reconnect Google Calendar" });
    expect(screen.getByRole("button", { name: "Test connection" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Disconnect" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect with Google" })).toBeNull();
  });
});

describe("a build with no Google client of its own", () => {
  it("explains it, lists the steps, and shows the paste form with one Save and grant access button", async () => {
    show("none", { info: NO_CLIENT });
    await screen.findByText(/no Google sign-in built in/);
    expect(screen.getByText(/Desktop app/)).toBeInTheDocument();
    expect(screen.getByLabelText("OAuth client id")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save and grant access" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect with Google" })).toBeNull();
    expect(screen.getByRole("link", { name: "Open the Google Cloud console" })).toHaveAttribute(
      "href",
      "https://console.cloud.google.com/apis/credentials",
    );
  });

  it("refuses an empty client in words, before asking Google anything", async () => {
    const grant = vi.fn(async () => true);
    const edit = show("none", { info: NO_CLIENT, grant });
    fireEvent.click(await screen.findByRole("button", { name: "Save and grant access" }));
    expect(edit.errorFor("gcal")).toBe("Marshal needs both a Google OAuth client id and secret.");
    expect(grant).not.toHaveBeenCalled();
  });
});
