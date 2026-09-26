import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { TerminalView } from "./TerminalView";
import { createTerminalState } from "./terminal-state";

/*
 * The terminal view's mock path (section S9): unchanged for a mock-only card (one with no
 * `daemonId`) even though S9 itself is switched, exactly the pattern every other dual-path
 * component in this app keeps (`DiffTab.tsx`, `ChatTab.tsx`). The daemon path is
 * `TerminalView.daemon.test.tsx`; `sync/card-view.test.ts` covers the sync module on its own.
 */

afterEach(cleanup);

describe("the mock path (a mock-only card)", () => {
  it("draws the fake terminal's own header and prompt, not the daemon's decoded text", () => {
    const card = M.S.cards[0];
    if (!card) throw new Error("the shared test store has no cards");
    const original = card.daemonId;
    card.daemonId = undefined;
    const terminal = createTerminalState();
    try {
      render(() => <TerminalView card={card} terminal={terminal} scrollRef={() => undefined} />);
      // `terminal-lines.ts`'s own header line, which the daemon path never draws.
      expect(screen.getByText(/^Resumed in /)).toBeInTheDocument();
      expect(screen.getByLabelText("Terminal input")).toBeInTheDocument();
    } finally {
      card.daemonId = original;
    }
  });

  it("still requires a non-empty, trimmed command, and sends it as a plain chat message", () => {
    const card = M.S.cards[0];
    if (!card) throw new Error("the shared test store has no cards");
    const original = card.daemonId;
    card.daemonId = undefined;
    const send = vi.spyOn(M, "send").mockImplementation(() => undefined);
    const terminal = createTerminalState();
    try {
      render(() => <TerminalView card={card} terminal={terminal} scrollRef={() => undefined} />);
      const input = screen.getByLabelText("Terminal input") as HTMLInputElement;
      fireEvent.submit(input.closest("form") as HTMLFormElement);
      expect(send).not.toHaveBeenCalled();
      fireEvent.input(input, { target: { value: "  " } });
      fireEvent.submit(input.closest("form") as HTMLFormElement);
      expect(send).not.toHaveBeenCalled();
      fireEvent.input(input, { target: { value: "hello" } });
      fireEvent.submit(input.closest("form") as HTMLFormElement);
      expect(send).toHaveBeenCalledWith(card.id, "hello");
    } finally {
      card.daemonId = original;
      send.mockRestore();
    }
  });

  it("logs a key bar press locally and sends nothing over the wire", async () => {
    const card = M.S.cards[0];
    if (!card) throw new Error("the shared test store has no cards");
    const original = card.daemonId;
    card.daemonId = undefined;
    const before = M.S.vw;
    M.setViewport(390, 844);
    const terminal = createTerminalState();
    try {
      render(() => <TerminalView card={card} terminal={terminal} scrollRef={() => undefined} />);
      fireEvent.click(screen.getByRole("button", { name: "Escape key" }));
      expect(terminal.keyLog()).toEqual(["Escape key"]);
      // The mock's own log line appears in the fake terminal text; nothing here can observe a wire
      // send at all, since a mock-only card's `M.terminalKey` is unreachable from this view.
      await vi.waitFor(() => expect(screen.getByText("[Escape key]")).toBeInTheDocument());
    } finally {
      card.daemonId = original;
      M.setViewport(before, M.S.vh);
    }
  });
});
