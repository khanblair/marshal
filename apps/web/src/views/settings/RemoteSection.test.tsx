import { cleanup, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { showSettings } from "./test-support";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

describe("Remote control section on the prototype", () => {
  it("is in the section list, and opens with its own heading", () => {
    showSettings("remote");
    expect(screen.getByRole("button", { name: "Remote control" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("heading", { name: "Remote control", level: 2 })).toBeInTheDocument();
  });

  it("shows the prototype's own devices, account and machine, and no phone address it cannot know", () => {
    showSettings("remote");
    expect(screen.getByText("Pixel 8")).toBeInTheDocument();
    expect(screen.getByText("ada@kolaborate.co")).toBeInTheDocument();
    expect(screen.getByText("marshal-laptop.tail3f2a.ts.net")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Phone address" })).toBeNull();
  });

  it("says so when there are no paired devices", () => {
    showSettings("remote");
    M.S.profile.devices = [];
    expect(screen.getByText("No paired devices.")).toBeInTheDocument();
  });

  it("has Pair a device and Check again", () => {
    showSettings("remote");
    expect(screen.getByRole("button", { name: "Pair a device" })).toBeInTheDocument();
    // While the daemon is asked, the button says so, and a prototype has no daemon to ask.
    expect(screen.getByRole("button", { name: /^Check/ })).toBeInTheDocument();
  });
});
