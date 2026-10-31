import type { AgentCatalog } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { golden } from "~/data/testing/golden";
import { M } from "~/mock";
import { SettingsView } from "./SettingsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/** The catalog the daemon answers with, written by the Go tests. */
const GOLDEN = golden<AgentCatalog>("agents");

beforeEach(() => {
  M.set({ settingsSection: "agents", toasts: [], agents: GOLDEN.agents });
});
afterEach(() => {
  vi.restoreAllMocks();
  cleanup();
});

describe("the Agents section", () => {
  it("lists the agents Marshal looks for, and leaves out Marshal's own agent", () => {
    render(() => <SettingsView />);
    for (const agent of GOLDEN.agents) {
      if (agent.kind === "builtin") {
        expect(screen.queryByText(agent.name)).not.toBeInTheDocument();
      } else {
        expect(screen.getByText(agent.name)).toBeInTheDocument();
      }
    }
  });

  it("saves the path that was typed, as the agent's program and with its on switch unchanged", () => {
    const save = vi.spyOn(M, "saveAgentSettings").mockResolvedValue(null);
    const claude = GOLDEN.agents.find((agent) => agent.kind === "claude");
    if (!claude) throw new Error("the golden catalog has no claude");
    render(() => <SettingsView />);

    fireEvent.click(screen.getAllByRole("button", { name: /Show details of/ })[0] as HTMLElement);
    fireEvent.input(screen.getByLabelText(/Program path/), {
      target: { value: "  /Users/pat/bin/claude  " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save path" }));

    expect(save).toHaveBeenCalledWith("claude", { path: "/Users/pat/bin/claude", off: claude.off });
  });

  it("turns an agent off with its switch and keeps its path", () => {
    const save = vi.spyOn(M, "saveAgentSettings").mockResolvedValue(null);
    render(() => <SettingsView />);

    fireEvent.click(screen.getAllByRole("button", { name: /Show details of/ })[0] as HTMLElement);
    fireEvent.click(screen.getByRole("switch", { name: /is on for cards/ }));

    expect(save).toHaveBeenCalledWith("claude", expect.objectContaining({ off: true }));
  });
});
