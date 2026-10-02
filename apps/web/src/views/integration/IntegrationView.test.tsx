import type { IntegrationState } from "@marshal/protocol";
import { cleanup, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { toMergeFlow } from "~/data/mappers/integration";
import { golden } from "~/data/testing/golden";
import { M } from "~/mock";
import { IntegrationView } from "./IntegrationView";

/*
 * The view over a store with no daemon, which is the mock's own: nothing is ever read, so it draws
 * the plain empty state. A flow put into the store by hand is drawn from the golden answer, with the
 * cards named as the daemon names them because the store has none of these.
 */
const wire = golden<IntegrationState>("integration-state");

beforeEach(() => {
  M.go("project", "web", "integration");
  M.set({ integration: {}, toasts: [] });
});
afterEach(cleanup);

const slot = (state: IntegrationState = wire) => ({
  flow: toMergeFlow(state),
  error: "",
  loading: false,
});

describe("IntegrationView with no daemon", () => {
  it("shows the plain empty state and reads nothing", () => {
    render(() => <IntegrationView />);
    expect(screen.getByText("Nothing is waiting to merge.")).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
    expect(M.S.integration?.web).toBeUndefined();
  });

  it("draws the header, the lanes the queue is in, and what was delivered", () => {
    M.set({ integration: { web: slot() } });
    render(() => <IntegrationView />);
    expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Merging into development");
    expect(screen.getByText("integrator")).toBeInTheDocument();
    expect(screen.getByText("Ahead by 2")).toBeInTheDocument();
    expect(screen.getByText("Merging web#12")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Pause merging" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open Integrator chat" })).toBeInTheDocument();

    const resolving = screen.getByRole("region", { name: "Resolving conflicts" });
    expect(within(resolving).getByText("Add login page")).toBeInTheDocument();
    expect(within(resolving).getByText("web#12")).toBeInTheDocument();
    const waiting = screen.getByRole("region", { name: "Waiting to merge" });
    expect(within(waiting).getByText("Fix search bug")).toBeInTheDocument();
    expect(within(waiting).getByText("Place 2 in line")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Testing" })).toBeNull();
    expect(screen.queryByRole("region", { name: "Landing in your folder" })).toBeNull();
    expect(screen.queryByText("Nothing is waiting to merge.")).toBeNull();

    const delivered = screen.getByRole("region", { name: "Delivered" });
    const row = within(delivered).getByRole("article", { name: "web#11 Rename config" });
    expect(within(row).getByText("Resolved 1 conflict")).toBeInTheDocument();
    expect(
      within(row).getByText("Both cards edited config.py; kept both settings."),
    ).toBeInTheDocument();
    expect(within(row).getByRole("button", { name: "Undo" })).toBeInTheDocument();
  });

  it("says nothing is waiting but still lists what was delivered, and offers no Undo when it cannot", () => {
    const delivered = wire.history.map((item) => ({ ...item, canUndo: false, resolved: 0 }));
    M.set({
      integration: {
        web: slot({ ...wire, state: "idle", currentCardId: "", queue: [], history: delivered }),
      },
    });
    render(() => <IntegrationView />);
    expect(screen.getByText("Nothing is waiting to merge.")).toBeInTheDocument();
    expect(screen.getByText("Idle")).toBeInTheDocument();
    const row = screen.getByRole("article", { name: "web#11 Rename config" });
    expect(within(row).queryByRole("button", { name: "Undo" })).toBeNull();
    expect(within(row).queryByText(/Resolved/)).toBeNull();
  });

  it("shows a paused Integrator with Resume merging, and why a waiting one stopped", () => {
    M.set({ integration: { web: slot({ ...wire, state: "paused" }) } });
    render(() => <IntegrationView />);
    expect(screen.getByText("Paused")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Resume merging" })).toBeInTheDocument();
    cleanup();
    M.set({
      integration: {
        web: slot({ ...wire, state: "waiting", message: "A conflict in config.py needs you." }),
      },
    });
    render(() => <IntegrationView />);
    expect(screen.getByText("Waiting for you")).toBeInTheDocument();
    expect(screen.getByText("A conflict in config.py needs you.")).toBeInTheDocument();
  });

  it("says a read is on its way, and offers Try again when it failed", () => {
    M.set({ integration: { web: { flow: null, error: "", loading: true } } });
    render(() => <IntegrationView />);
    expect(screen.getByRole("status")).toHaveTextContent("Reading the merge queue.");
    cleanup();
    M.set({
      integration: { web: { flow: null, error: "The merge queue is busy.", loading: false } },
    });
    render(() => <IntegrationView />);
    expect(screen.getByText("The merge queue is busy.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });
});
