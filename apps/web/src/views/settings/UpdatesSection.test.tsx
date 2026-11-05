import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { createStore } from "solid-js/store";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { UpdateSnapshot } from "~/features/updates/update-store";
import { updates } from "~/features/updates/updates";
import { UpdatesSection } from "./UpdatesSection";

vi.mock("~/features/updates/updates", () => {
  const [state, set] = createStore<UpdateSnapshot>({
    status: "web",
    currentVersion: "",
    downloaded: 0,
  });
  return {
    updates: { state, set, check: vi.fn(), install: vi.fn(), restart: vi.fn() },
    openReleases: vi.fn(),
  };
});
const set = (value: Partial<UpdateSnapshot>) =>
  (updates as unknown as { set: (value: Partial<UpdateSnapshot>) => void }).set(value);
afterEach(() => {
  cleanup();
  set({
    status: "web",
    currentVersion: "",
    version: null,
    notes: null,
    downloaded: 0,
    error: null,
  });
  vi.clearAllMocks();
});

describe("Updates settings", () => {
  it("explains browser support without offering a fake install", () => {
    render(() => <UpdatesSection />);
    expect(screen.getByText(/Open Marshal on your computer/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Download and install" })).toBeNull();
  });
  it("shows versions, safely rendered notes, and installs only on click", () => {
    set({
      status: "available",
      currentVersion: "1.0.0",
      version: "1.1.0",
      notes: "<script>release notes</script>",
    });
    const { container } = render(() => <UpdatesSection />);
    expect(screen.getByText("Installed version 1.0.0")).toBeInTheDocument();
    expect(screen.getByText("Version 1.1.0")).toBeInTheDocument();
    expect(container.querySelector("script")).toBeNull();
    expect(updates.install).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Download and install" }));
    expect(updates.install).toHaveBeenCalledTimes(1);
  });
  it("shows indeterminate download progress and restart action", () => {
    set({ status: "downloading", total: null });
    render(() => <UpdatesSection />);
    expect(screen.getByRole("progressbar", { name: "Update download" })).not.toHaveAttribute(
      "value",
    );
    set({ status: "installed" });
    fireEvent.click(screen.getByRole("button", { name: "Restart Marshal" }));
    expect(updates.restart).toHaveBeenCalledTimes(1);
    expect(screen.getByText(/Finish any running work/)).toBeInTheDocument();
  });
});
