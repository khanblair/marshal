import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Integration } from "~/mock";
import { createEditState } from "./edit-state";
import { GoogleConnectDialog } from "./GoogleConnectDialog";

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
const OWN_CLIENT = async () => ({ bundled: true, own: true });

function show(
  st: Integration["st"],
  extra: Partial<Parameters<typeof GoogleConnectDialog>[0]> = {},
) {
  const edit = createEditState();
  edit.open("gcal");
  render(() => (
    <GoogleConnectDialog
      integration={row(st)}
      edit={edit}
      info={ONE_CLICK}
      refresh={async () => undefined}
      {...extra}
    />
  ));
  return edit;
}

const tab = (name: string) => screen.getByRole("tab", { name });

describe("the two tabs", () => {
  it("opens on Sign in with Google when Marshal's own client is built in, and pastes nothing", async () => {
    show("none");
    await screen.findByRole("button", { name: "Connect with Google" });
    expect(tab("Sign in with Google")).toHaveAttribute("aria-selected", "true");
    expect(tab("Use my own client")).toHaveAttribute("aria-selected", "false");
    expect(screen.queryByLabelText("OAuth client id")).toBeNull();
    expect(screen.getByText(/It can only read, never change anything/)).toBeInTheDocument();
  });

  it("keeps the paste form, its steps and the console link on the other tab", async () => {
    show("none");
    await screen.findByRole("button", { name: "Connect with Google" });
    fireEvent.click(tab("Use my own client"));
    expect(screen.getByLabelText("OAuth client id")).toBeInTheDocument();
    expect(screen.getByText(/Desktop app/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open the Google Cloud console" })).toHaveAttribute(
      "href",
      "https://console.cloud.google.com/apis/credentials",
    );
    expect(screen.getByRole("button", { name: "Save and grant access" })).toBeInTheDocument();
  });

  it("opens on the person's own client when they saved one", async () => {
    show("connected", { info: OWN_CLIENT });
    await waitFor(() => expect(tab("Use my own client")).toHaveAttribute("aria-selected", "true"));
  });

  it("closes from the Close button", async () => {
    const edit = show("none");
    await screen.findByRole("button", { name: "Connect with Google" });
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(edit.id()).toBeNull();
  });
});

describe("Connect with Google", () => {
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

  it("offers Reconnect, Test and Disconnect on either tab once connected, and no Connect button", async () => {
    show("connected");
    await screen.findByRole("button", { name: "Reconnect Google Calendar" });
    expect(screen.getByRole("button", { name: "Test connection" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Disconnect" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect with Google" })).toBeNull();
    fireEvent.click(tab("Use my own client"));
    expect(screen.getByRole("button", { name: "Disconnect" })).toBeInTheDocument();
  });
});

describe("a build with no Google client of its own", () => {
  it("opens on the paste tab, with the steps and one Save and grant access button", async () => {
    show("none", { info: NO_CLIENT });
    await waitFor(() => expect(tab("Use my own client")).toHaveAttribute("aria-selected", "true"));
    expect(screen.getByText(/Desktop app/)).toBeInTheDocument();
    expect(screen.getByLabelText("OAuth client id")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save and grant access" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect with Google" })).toBeNull();
  });

  it("says on the sign-in tab that this build has no Google sign-in, and points to the other tab", async () => {
    show("none", { info: NO_CLIENT });
    await waitFor(() => expect(tab("Use my own client")).toHaveAttribute("aria-selected", "true"));
    fireEvent.click(tab("Sign in with Google"));
    expect(await screen.findByText(/no Google sign-in built in/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect with Google" })).toBeNull();
  });

  it("refuses an empty client in words, before asking Google anything", async () => {
    const grant = vi.fn(async () => true);
    const edit = show("none", { info: NO_CLIENT, grant });
    fireEvent.click(await screen.findByRole("button", { name: "Save and grant access" }));
    expect(edit.errorFor("gcal")).toBe("Marshal needs both a Google OAuth client id and secret.");
    expect(grant).not.toHaveBeenCalled();
  });
});
