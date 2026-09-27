// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (S5a and S13 are the daemon's).
import { daemon, resetDaemonCards, resetStoreCards } from "~/testing/daemon-cards-store";
import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resetShell } from "~/app/shell-test-utils";
import { type Card, M } from "~/mock";
import type { CardKey } from "~/mock/card-key";
import { PreviewTab } from "./PreviewTab";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * The Preview tab against the fake daemon (section S13, docs/backend-checklist.md B6.6 and B6.7).
 * The daemon owns the state, the address, the port, and the screenshots, so the tab only draws what
 * it last answered. Nothing here launches a browser: the daemon's own shooter sits behind a fake, and
 * the tab's Start, Stop, and Take screenshot go through the same routes and events a person's do.
 */

const WEB: CardKey = "web#118";
const MOBILE: CardKey = "mobile#210";

/** The daemon's own dev command for a project Marshal can preview. */
const DEV_COMMAND = "pnpm dev";

/** The daemon's own sentence for a project it has no dev command for (daemon/internal/preview). */
const NO_COMMAND =
  "This project has no dev command, so Marshal does not know how to start it. Add one in project settings.";

beforeEach(() => {
  resetDaemonCards();
  resetStoreCards();
  resetShell();
  daemon.calls.splice(0, daemon.calls.length);
  // A preview is the daemon's, so a test starts from a daemon that holds none and a store that has
  // read none: the store keeps the last daemon's preview per card, and a stale one would draw a
  // running server the daemon does not have.
  daemon.previews.previews.clear();
  daemon.previews.nextPort = 5100;
  M.S.preview = undefined;
  // The web project can be previewed; the mobile one cannot, and the daemon refuses its start.
  for (const project of daemon.projects) {
    project.devCommand = project.id === "web" ? DEV_COMMAND : "";
  }
});
afterEach(cleanup);

const card = (key: CardKey): Card => {
  const one = M.card(key);
  if (!one) throw new Error(`no card ${key}`);
  return one;
};
const daemonId = (key: CardKey): string => encodeURIComponent(card(key).daemonId ?? "");
const route = (key: CardKey, action: string): string =>
  `POST /v1/cards/${daemonId(key)}/preview/${action}`;
const status = (): HTMLElement => screen.getByText(/Running|Starting|Stopped/);
const said = (): string => M.S.toasts.at(-1)?.msg ?? "";

/** A shot's bytes are drawn from an object URL, which jsdom does not make; the tab only needs one. */
beforeEach(() => {
  URL.createObjectURL = vi.fn(() => "blob:test/shot");
  URL.revokeObjectURL = vi.fn();
});

describe("a card's preview on the daemon", () => {
  it("starts from stopped, with the daemon's own sentence and nothing running", () => {
    render(() => <PreviewTab card={card(WEB)} />);
    expect(screen.getByText("No server running")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Start preview" })).toBeInTheDocument();
    expect(screen.getByText(/^Stopped\./)).toBeInTheDocument();
    // Nothing has been read for the card yet, so nothing is drawn that only a running server has.
    expect(screen.queryByText("Screenshots")).not.toBeInTheDocument();
  });

  it("runs the project's dev command and draws the daemon's address, port, and command", async () => {
    render(() => <PreviewTab card={card(WEB)} />);
    fireEvent.click(screen.getByRole("button", { name: "Start preview" }));
    await vi.waitFor(() => expect(status()).toHaveTextContent("Running pnpm dev on port 5100"));
    expect(screen.getByText("http://127.0.0.1:5100")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Stop preview" })).toBeInTheDocument();
    expect(daemon.routes()).toContain(route(WEB, "start"));
  });

  it("stops it again, and the daemon says it is stopped", async () => {
    render(() => <PreviewTab card={card(WEB)} />);
    fireEvent.click(screen.getByRole("button", { name: "Start preview" }));
    await vi.waitFor(() => expect(status()).toHaveTextContent("Running pnpm dev on port 5100"));
    fireEvent.click(screen.getByRole("button", { name: "Stop preview" }));
    await vi.waitFor(() => expect(status()).toHaveTextContent(/^Stopped\./));
    expect(screen.getByText("No server running")).toBeInTheDocument();
    expect(daemon.routes()).toContain(route(WEB, "stop"));
  });

  it("shows the daemon's own sentence when it refuses to start a project with no dev command", async () => {
    render(() => <PreviewTab card={card(MOBILE)} />);
    fireEvent.click(screen.getByRole("button", { name: "Start preview" }));
    await vi.waitFor(() => expect(said()).toBe(NO_COMMAND));
    // A start that was refused is not a spinner that never ends: the tab is still stopped.
    expect(screen.getByText("No server running")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Start preview" })).toBeInTheDocument();
  });

  it("takes one screenshot, draws its bytes, and leaves the other half to take", async () => {
    render(() => <PreviewTab card={card(WEB)} />);
    fireEvent.click(screen.getByRole("button", { name: "Start preview" }));
    await vi.waitFor(() => expect(status()).toHaveTextContent("Running pnpm dev on port 5100"));
    // Both halves are placeholders with their own button until the daemon takes one.
    const take = screen.getAllByRole("button", { name: "Take screenshot" });
    expect(take).toHaveLength(2);
    fireEvent.click(take[0] as HTMLElement);
    await vi.waitFor(() => expect(screen.getByAltText("Before screenshot")).toBeInTheDocument());
    expect(daemon.bodies(`POST /v1/cards/${daemonId(WEB)}/preview/shots`)).toEqual([
      { kind: "before" },
    ]);
    // Only the half the daemon took is drawn; the other still offers to take one.
    expect(screen.queryAllByRole("button", { name: "Take screenshot" })).toHaveLength(1);
    expect(screen.getByAltText("Before screenshot")).toBeInTheDocument();
  });

  it("follows a preview.state_changed the daemon publishes, without the tab asking again", async () => {
    render(() => <PreviewTab card={card(WEB)} />);
    // A server that settles into running elsewhere (another device, or the daemon's own start timer)
    // arrives on the card's own topic, and the tab draws it.
    daemon.emit(`card:${card(WEB).daemonId}`, "preview.state_changed", {
      preview: {
        cardId: card(WEB).daemonId,
        state: "running",
        url: "http://127.0.0.1:5199",
        port: 5199,
        command: DEV_COMMAND,
        startedAt: new Date().toISOString(),
        error: "",
        shots: [],
      },
    });
    await vi.waitFor(() => expect(status()).toHaveTextContent("Running pnpm dev on port 5199"));
    expect(screen.getByText("http://127.0.0.1:5199")).toBeInTheDocument();
  });
});
