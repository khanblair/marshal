// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (S5a, S7c, and S9 are the daemon's).
import { daemon, resetDaemonCards, resetStoreCards } from "~/testing/daemon-cards-store";
import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { applyTerminalFrame } from "~/sync/card-view";
import { type Card, M } from "~/mock";
import type { CardKey } from "~/mock/card-key";
import { contextOf } from "~/testing/test-store";
import { createTerminalState } from "./terminal-state";
import { TerminalView } from "./TerminalView";

/*
 * The terminal view's daemon path (section S9, docs/architecture.md 4.2, 4.3, 11.2), against the
 * fake daemon that answers as the real one does. `sync/card-view.test.ts` covers the sync module's
 * own logic (switching, decoding, buffering) in full; this is the component's own seam: it reads
 * `M.terminalText`, and it sends `M.terminalSend`/`M.terminalKey` for what a person types or
 * presses, exactly the pattern `DiffTab.tsx`/`ChatTab.tsx`'s dual-path components already use.
 */

const WORKING: CardKey = "api#41";

beforeEach(() => {
  resetDaemonCards();
  resetStoreCards();
  daemon.calls.splice(0, daemon.calls.length);
});
afterEach(cleanup);

const card = (key: CardKey): Card => {
  const one = M.card(key);
  if (!one) throw new Error(`no card ${key}`);
  return one;
};

/** Opens the card and waits for its own topic to actually be subscribed, the way `card-view.test.ts`
 * does: the fake daemon's terminal router checks a socket's latest hello before it answers a
 * terminal message, the same way the real one does. */
async function openAndFollow(key: CardKey): Promise<void> {
  M.S.openId = key;
  const topic = `card:${card(key).daemonId}`;
  await vi.waitFor(() => {
    const hello = daemon.sockets
      .last()
      .hellos()
      .findLast((one) => one.type === "hello");
    expect(Array.isArray(hello?.subscribe) && hello.subscribe.includes(topic)).toBe(true);
  });
}

function renderTerminal(key: CardKey) {
  // `terminal` is created once and passed down as the same reference, not called inline in the
  // JSX prop position: Solid wraps a component prop expression in a getter for reactivity, and
  // `createTerminalState()` inline there would hand a fresh, empty `keyLog` to every read of it.
  const terminal = createTerminalState();
  const found = card(key);
  return render(() => (
    <TerminalView card={found} terminal={terminal} scrollRef={() => undefined} />
  ));
}

describe("the daemon path", () => {
  it("renders the daemon's decoded, escape-stripped text instead of the fake terminal lines", async () => {
    await openAndFollow(WORKING);
    const daemonId = card(WORKING).daemonId as string;
    applyTerminalFrame(
      contextOf(M),
      {
        kind: "terminal.screen",
        frame: {
          type: "terminal.screen",
          cardId: daemonId,
          cols: 120,
          rows: 32,
          throughSeq: 1,
          data: btoa("\x1b[2Jready\r\n"),
        },
      },
      daemonId,
    );
    renderTerminal(WORKING);
    await vi.waitFor(() => expect(screen.getByText("ready", { exact: false })).toBeInTheDocument());
    expect(screen.queryByText(/session [0-9a-f]+a2f/)).not.toBeInTheDocument();
  });

  it("sends what is typed as terminal.input, with the trailing line end, not trimmed", async () => {
    await openAndFollow(WORKING);
    const daemonId = card(WORKING).daemonId as string;
    renderTerminal(WORKING);
    const input = screen.getByLabelText("Terminal input") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "ls -la" } });
    fireEvent.submit(input.closest("form") as HTMLFormElement);
    await vi.waitFor(() =>
      expect(daemon.terminalInputs(daemonId).at(-1)).toEqual({
        type: "terminal.input",
        cardId: daemonId,
        data: "ls -la\r",
      }),
    );
    expect(input.value).toBe("");
  });

  it("sends a bare line end for an empty submit, the only way this view answers a bare Enter prompt", async () => {
    await openAndFollow(WORKING);
    const daemonId = card(WORKING).daemonId as string;
    renderTerminal(WORKING);
    const input = screen.getByLabelText("Terminal input") as HTMLInputElement;
    fireEvent.submit(input.closest("form") as HTMLFormElement);
    await vi.waitFor(() =>
      expect(daemon.terminalInputs(daemonId).at(-1)).toEqual({
        type: "terminal.input",
        cardId: daemonId,
        data: "\r",
      }),
    );
  });

  it("sends the key bar's press as the wire's own named key", async () => {
    await openAndFollow(WORKING);
    const daemonId = card(WORKING).daemonId as string;
    M.setViewport(390, 844);
    renderTerminal(WORKING);
    fireEvent.click(screen.getByRole("button", { name: "Escape key" }));
    await vi.waitFor(() =>
      expect(daemon.terminalInputs(daemonId).at(-1)).toEqual({
        type: "terminal.input",
        cardId: daemonId,
        key: "esc",
      }),
    );
  });
});
