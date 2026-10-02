import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { M } from "~/mock";
import { GoogleNotice } from "./GoogleNotice";

beforeEach(() => {
  M.set({ calGoogle: { known: false, connected: false, error: "", stale: false } });
});
afterEach(cleanup);

const google = (over: Partial<ReturnType<() => typeof M.S.calGoogle>>) =>
  M.set({ calGoogle: { known: true, connected: true, error: "", stale: false, ...over } });

describe("the Google notice", () => {
  it("says nothing before the daemon has answered, so it never claims what it has not asked", () => {
    render(() => <GoogleNotice />);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("says nothing while Google is connected and reads fine", () => {
    google({});
    render(() => <GoogleNotice />);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("says Google Calendar is not connected, and Connect goes to the Integrations settings", () => {
    google({ connected: false });
    render(() => <GoogleNotice />);
    expect(screen.getByRole("status")).toHaveTextContent("Google Calendar is not connected");
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    expect(M.S.route.page).toBe("settings");
    expect(M.S.settingsSection).toBe("integrations");
  });

  it("gives the daemon's own sentence when Google could not be read", () => {
    google({
      error: "Google no longer accepts Marshal's access. Reconnect Google Calendar in Settings.",
    });
    render(() => <GoogleNotice />);
    expect(screen.getByRole("status")).toHaveTextContent("Reconnect Google Calendar in Settings");
    expect(screen.getByRole("button", { name: "Open settings" })).toBeInTheDocument();
  });
});
