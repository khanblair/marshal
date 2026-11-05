import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resetShell } from "~/app/shell-test-utils";
import { M } from "~/mock";
import { UpdateNotice } from "./UpdateNotice";
import { updates } from "./updates";

vi.mock("./updates", () => ({
  updates: {
    state: { status: "available", version: "1.1.0" },
    initialize: vi.fn(),
    refresh: vi.fn(),
  },
}));
beforeEach(() => {
  resetShell();
  vi.clearAllMocks();
});
afterEach(cleanup);

describe("automatic update prompt", () => {
  it("checks automatically and opens Updates without starting an installation", () => {
    render(() => <UpdateNotice />);
    expect(updates.initialize).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("status")).toHaveTextContent("Marshal 1.1.0 is available.");
    fireEvent.click(screen.getByRole("button", { name: "View update" }));
    expect(M.S.route.page).toBe("settings");
    expect(M.S.settingsSection).toBe("updates");
  });
  it("lets the person defer an update without taking focus", () => {
    render(() => <UpdateNotice />);
    fireEvent.click(screen.getByRole("button", { name: "Remind me later" }));
    expect(screen.queryByRole("status")).toBeNull();
  });
});
