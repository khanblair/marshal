import type { ClientFrame, Card as WireCard, Event as WireEvent } from "@marshal/protocol";
import { createRoot } from "solid-js";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type SectionId, type SectionStatus, sectionStatus } from "~/data/sections";
import {
  buildTerminalInput,
  buildTerminalResize,
  buildTerminalSnapshot,
} from "~/data/stream-frames";
import type { Marshal } from "~/mock";
import type { Ctx } from "~/mock/context";
import { cardId, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import {
  applyTerminalFrame,
  applyTerminalOutputEvent,
  followOpenCardTerminal,
  sendTerminalKey,
  sendTerminalText,
  stripAnsi,
  switchView,
  terminalTextOf,
} from "./card-view";

// Section S9 (docs/architecture.md 4.2, 4.3, 11.2): the terminal view's client vertical slice. The
// cards (S5a) are the daemon's, and so is the view switch.
const ON_DAEMON = { ...sectionStatus, S5a: "daemon" as const, S9: "daemon" as const };
const STILL_MOCK = { ...sectionStatus, S5a: "daemon" as const, S9: "mock" as const };

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

/** A working card with a live session and terminal, in the chat view (the daemon's own default). */
const api41 = (fields: Partial<WireCard> = {}): WireCard =>
  wireCard({
    projectId: "api",
    number: 41,
    title: "Fix token refresh",
    state: "working",
    session: "awake",
    viewMode: "chat",
    ...fields,
  });

const route = (): string => `POST /v1/cards/${encodeURIComponent(cardId(41))}/view`;

interface Fixture {
  M: Marshal;
  ctx: Ctx;
  d: FakeDaemon;
}

/** Waits until the app has stopped asking for boards, the same wait `card-hold.test.ts` uses, so a
 * Resync's own reload never lands on top of a change a test just made. */
async function settled(d: FakeDaemon): Promise<void> {
  let last = -1;
  await vi.waitFor(() => {
    const loads = d.routes().filter((one) => one.endsWith("/board")).length;
    const steady = loads > 0 && loads === last;
    last = loads;
    expect(steady).toBe(true);
  });
}

async function setup(
  card: WireCard = api41(),
  sections: Readonly<Record<SectionId, SectionStatus>> = ON_DAEMON,
): Promise<Fixture> {
  const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, cards: [card] });
  daemon = d;
  const M = createTestMarshal({ data: d.data, sections });
  await d.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  await settled(d);
  return { M, ctx: contextOf(M), d };
}

const toasts = (M: Marshal): string[] => M.S.toasts.map((one) => one.msg);

describe("stripAnsi", () => {
  it("leaves plain text unchanged", () => {
    expect(stripAnsi("ready\r\n")).toBe("ready\r\n");
  });

  it("removes a color-code sequence", () => {
    expect(stripAnsi("\x1b[31mred\x1b[0m plain")).toBe("red plain");
  });

  it("removes a cursor-move sequence, matching the daemon's own golden screen", () => {
    // daemon/testdata/golden/terminal-screen.json's data, decoded: a clear, a cursor home, "ready\r\n".
    expect(stripAnsi("\x1b[2J\x1b[Hready\r\n")).toBe("ready\r\n");
  });

  it("leaves a carriage-return-only line readable", () => {
    expect(stripAnsi("progress: 50%\rprogress: 100%")).toBe("progress: 50%\rprogress: 100%");
  });

  it("strips an OSC sequence (a window title) too", () => {
    expect(stripAnsi("\x1b]0;my title\x07after")).toBe("after");
  });
});

describe("switchView", () => {
  it("switches the open card to the daemon's answer, showing switching meanwhile", async () => {
    const { ctx, d } = await setup();
    ctx.S.openId = "api#41";
    expect(ctx.S.mode).toBe("chat");
    const pending = switchView(ctx, "api#41", "terminal");
    expect(ctx.S.switching).toBe("terminal");
    await pending;
    expect(ctx.S.mode).toBe("terminal");
    expect(ctx.S.switching).toBe(false);
    expect(d.routes()).toContain(route());
    expect(d.bodies(route())).toEqual([{ mode: "terminal" }]);
    // The store's own card, not just `S.mode`: `followOpenCardTerminal`'s effect reads
    // `card.viewMode` and would otherwise write `S.mode` straight back to a stale value the next
    // time anything re-runs it, since the real daemon gives no ordering guarantee that a
    // `card.updated` announcing the new `viewMode` arrives before (or ever, for an awake-to-awake
    // switch) this answer does.
    expect(ctx.S.cards.find((one) => one.id === "api#41")?.viewMode).toBe("terminal");
  });

  it("is a no-op when the card is already in that mode", async () => {
    const { ctx, d } = await setup();
    ctx.S.openId = "api#41";
    await switchView(ctx, "api#41", "chat");
    expect(d.routes().filter((one) => one.endsWith("/view"))).toEqual([]);
  });

  it("is a no-op for a card the store does not know, or one with no daemon id", async () => {
    const { ctx, d } = await setup();
    ctx.S.openId = "api#41";
    await switchView(ctx, "api#9999", "terminal");
    expect(d.routes().filter((one) => one.endsWith("/view"))).toEqual([]);
  });

  it("shows Marshal is not connected when there is no daemon", async () => {
    const M = createTestMarshal({ sections: ON_DAEMON });
    const ctx = contextOf(M);
    ctx.S.cards.push({ ...M.S.cards[0]!, id: "api#41", daemonId: cardId(41) });
    ctx.S.openId = "api#41";
    await switchView(ctx, "api#41", "terminal");
    expect(toasts(M)).toContain("Marshal is not connected to its daemon.");
    expect(ctx.S.mode).toBe("chat");
  });

  it("refuses a card with no agent running, the fake's own rule", async () => {
    const { M, ctx } = await setup(api41({ session: null }));
    ctx.S.openId = "api#41";
    await switchView(ctx, "api#41", "terminal");
    expect(toasts(M)).toContain("This card has no agent running. Start the card first.");
    expect(ctx.S.mode).toBe("chat");
    expect(ctx.S.switching).toBe(false);
  });

  // The daemon's own sentences (daemon/internal/session/view.go), driven through `refuseNext` since
  // this app never branches on which reason a switch was refused for, only on the message.
  const REFUSALS = [
    [
      "view_turn_running",
      "The agent is in the middle of a turn. Wait for it to finish, then switch views.",
    ],
    [
      "view_holding_messages",
      "This card has a message waiting for you to resume it. Resume the card first.",
    ],
    ["view_no_terminal", "This agent has no terminal view. Stay in the chat view."],
    ["view_switching", "This card is switching views. Try again in a moment."],
    [
      "view_terminal_active",
      "This card is in terminal view. Type in the terminal, or switch to chat view to send a message.",
    ],
    ["view_cannot_resume", "Marshal could not pick this session back up. The card now needs you."],
  ] as const;

  it.each(REFUSALS)(
    "shows the daemon's own sentence for %s, and leaves mode and switching as they were",
    async (_reason, message) => {
      const { M, ctx, d } = await setup();
      ctx.S.openId = "api#41";
      d.refuseNext(route(), 422, "refused", message);
      await switchView(ctx, "api#41", "terminal");
      expect(toasts(M)).toContain(message);
      expect(ctx.S.mode).toBe("chat");
      expect(ctx.S.switching).toBe(false);
    },
  );

  it("does not send a second request while the first is still in flight", async () => {
    const { ctx, d } = await setup();
    ctx.S.openId = "api#41";
    // `switchView`'s own guard (`S.switching`, set synchronously before the first request goes out)
    // already stops a second call from reaching the daemon at all, so nothing needs to be held.
    const first = switchView(ctx, "api#41", "terminal");
    const second = switchView(ctx, "api#41", "terminal");
    await Promise.all([first, second]);
    expect(ctx.S.mode).toBe("terminal");
    expect(d.routes().filter((one) => one.endsWith("/view"))).toHaveLength(1);
  });
});

describe("followOpenCardTerminal", () => {
  it("does nothing while S9 is still the mock's, and its resend is a no-op", async () => {
    const { ctx } = await setup(api41({ viewMode: "terminal" }), STILL_MOCK);
    const sent: ClientFrame[] = [];
    const stream = { sendTerminal: (f: ClientFrame) => sent.push(f) };
    const dispose = createRoot((stop) => {
      const resend = followOpenCardTerminal(ctx, daemon!.data.api, stream);
      ctx.S.openId = "api#41";
      resend();
      return stop;
    });
    expect(sent).toEqual([]);
    dispose();
  });

  it("sends a resize and a snapshot, and syncs S.mode, when the open card shows a real terminal", async () => {
    const { ctx } = await setup(api41({ viewMode: "terminal" }));
    const sent: ClientFrame[] = [];
    const stream = { sendTerminal: (f: ClientFrame) => sent.push(f) };
    const dispose = createRoot((stop) => {
      followOpenCardTerminal(ctx, daemon!.data.api, stream);
      return stop;
    });
    expect(sent).toEqual([]);
    ctx.S.openId = "api#41";
    await vi.waitFor(() => expect(sent.length).toBeGreaterThan(0));
    expect(sent).toEqual([
      buildTerminalResize(cardId(41), 120, 32),
      buildTerminalSnapshot(cardId(41)),
    ]);
    expect(ctx.S.mode).toBe("terminal");
    dispose();
  });

  it("sends nothing for a card stored in the chat view", async () => {
    const { ctx } = await setup(api41({ viewMode: "chat" }));
    const sent: ClientFrame[] = [];
    const stream = { sendTerminal: (f: ClientFrame) => sent.push(f) };
    const dispose = createRoot((stop) => {
      followOpenCardTerminal(ctx, daemon!.data.api, stream);
      return stop;
    });
    ctx.S.openId = "api#41";
    await vi.waitFor(() => expect(ctx.S.openId).toBe("api#41"));
    expect(sent).toEqual([]);
    expect(ctx.S.mode).toBe("chat");
    dispose();
  });

  it("asks again when its resend function is called, for a resync or a reconnection", async () => {
    const { ctx } = await setup(api41({ viewMode: "terminal" }));
    const sent: ClientFrame[] = [];
    const stream = { sendTerminal: (f: ClientFrame) => sent.push(f) };
    let resend: () => void = () => undefined;
    const dispose = createRoot((stop) => {
      resend = followOpenCardTerminal(ctx, daemon!.data.api, stream);
      return stop;
    });
    ctx.S.openId = "api#41";
    await vi.waitFor(() => expect(sent.length).toBeGreaterThan(0));
    sent.length = 0;
    resend();
    // The resend is deferred a microtask (so it never races ahead of the Hello a real reconnect
    // sends first; see `card-view.ts`'s own comment), so it is not on the wire synchronously here.
    await vi.waitFor(() => expect(sent.length).toBeGreaterThan(0));
    expect(sent).toEqual([
      buildTerminalResize(cardId(41), 120, 32),
      buildTerminalSnapshot(cardId(41)),
    ]);
    dispose();
  });

  it("sends nothing while a switch is in flight", async () => {
    const { ctx } = await setup(api41({ viewMode: "terminal" }));
    const sent: ClientFrame[] = [];
    const stream = { sendTerminal: (f: ClientFrame) => sent.push(f) };
    ctx.S.switching = "terminal";
    const dispose = createRoot((stop) => {
      followOpenCardTerminal(ctx, daemon!.data.api, stream);
      return stop;
    });
    ctx.S.openId = "api#41";
    await vi.waitFor(() => expect(ctx.S.openId).toBe("api#41"));
    expect(sent).toEqual([]);
    dispose();
  });

  it("keeps the hello ahead of the resend on a real reconnect, against the real event stream", async () => {
    // Unlike the tests above (a stub `stream`), this drives the real socket through `startSync`
    // (already running inside `setup()`), because the bug this guards is an ordering issue in
    // `event-stream.ts`'s own `opened()`: it fires `onReconnected` (which `sync/index.ts` wires to
    // this module's resend) before it sends the Hello that (re)subscribes the card's own topic. A
    // resend that raced ahead of that Hello would be answered the way the daemon answers any
    // message for a topic the connection does not yet follow: an error frame, and the connection
    // drops — this fake's own `endForBadTopic` reproduces exactly that.
    const { ctx, d } = await setup(api41({ viewMode: "terminal" }));
    const topic = `card:${cardId(41)}`;
    ctx.S.openId = "api#41";
    await vi.waitFor(() => {
      const hello = d.sockets
        .last()
        .hellos()
        .findLast((one) => one.type === "hello");
      expect(Array.isArray(hello?.subscribe) && hello.subscribe.includes(topic)).toBe(true);
    });
    await vi.waitFor(() => {
      expect(
        d.sockets
          .last()
          .hellos()
          .some((one) => one.type === "terminal.snapshot"),
      ).toBe(true);
    });
    d.sockets.last().drop();
    await vi.waitFor(() => expect(d.sockets.all.length).toBeGreaterThan(1), { timeout: 3000 });
    const next = d.sockets.last();
    next.accept();
    await vi.waitFor(() => {
      expect(next.hellos().some((one) => one.type === "terminal.snapshot")).toBe(true);
    });
    const msgs = next.hellos();
    const helloIndex = msgs.findIndex((one) => one.type === "hello");
    const snapshotIndex = msgs.findIndex((one) => one.type === "terminal.snapshot");
    expect(helloIndex).toBeGreaterThanOrEqual(0);
    expect(snapshotIndex).toBeGreaterThan(helloIndex);
    // The fake's own topic-follow check never fired, so the new socket was never dropped for it.
    expect(next.closedWith).toBeNull();
  });
});

describe("applying a card's terminal frames and output events", () => {
  const DAEMON_ID = cardId(41);
  const KEY = "api#41";

  const toBase64 = (text: string): string => btoa(text);
  const screenFrame = (throughSeq: number, text: string, id = DAEMON_ID) => ({
    kind: "terminal.screen" as const,
    frame: {
      type: "terminal.screen" as const,
      cardId: id,
      cols: 120,
      rows: 32,
      throughSeq,
      data: toBase64(text),
    },
  });
  const outputEvent = (seq: number, text: string, id = DAEMON_ID): WireEvent => ({
    seq,
    topic: `card:${id}`,
    type: "session.terminal_output",
    at: "2026-09-26T12:00:00.000Z",
    data: { cardId: id, data: toBase64(text) },
  });

  it("paints a screen's data, decoded and escape-stripped", async () => {
    const { ctx } = await setup();
    applyTerminalFrame(ctx, screenFrame(5, "\x1b[2Jready\r\n"), DAEMON_ID);
    expect(terminalTextOf(ctx, KEY)).toBe("ready\r\n");
  });

  it("applies only output events whose number is higher than the screen's throughSeq", async () => {
    const { ctx } = await setup();
    applyTerminalFrame(ctx, screenFrame(5, "ready\r\n"), DAEMON_ID);
    applyTerminalOutputEvent(ctx, outputEvent(5, "IGNORED (the screen's own number)"));
    applyTerminalOutputEvent(ctx, outputEvent(3, "IGNORED (older than the screen)"));
    expect(terminalTextOf(ctx, KEY)).toBe("ready\r\n");
    applyTerminalOutputEvent(ctx, outputEvent(6, "more\r\n"));
    expect(terminalTextOf(ctx, KEY)).toBe("ready\r\nmore\r\n");
  });

  it("drops an output event before any screen has arrived for that card", async () => {
    const { ctx } = await setup();
    applyTerminalOutputEvent(ctx, outputEvent(1, "too early"));
    expect(terminalTextOf(ctx, KEY)).toBe("");
  });

  it("replaces the buffer outright on a fresh screen, rather than appending to the old one", async () => {
    const { ctx } = await setup();
    applyTerminalFrame(ctx, screenFrame(5, "first window\r\n"), DAEMON_ID);
    applyTerminalFrame(ctx, screenFrame(9, "second window\r\n"), DAEMON_ID);
    expect(terminalTextOf(ctx, KEY)).toBe("second window\r\n");
  });

  it("bounds the buffer so a chatty program cannot grow it forever", async () => {
    const { ctx } = await setup();
    applyTerminalFrame(ctx, screenFrame(0, ""), DAEMON_ID);
    const CHUNK = "x".repeat(1000);
    const CHUNKS = 100; // 100 KiB of plain text, well past the 64 KiB bound.
    for (let seq = 1; seq <= CHUNKS; seq += 1) {
      applyTerminalOutputEvent(ctx, outputEvent(seq, CHUNK));
    }
    const MAX_TEXT_LENGTH = 64 * 1024;
    expect(terminalTextOf(ctx, KEY).length).toBeLessThanOrEqual(MAX_TEXT_LENGTH);
    expect(terminalTextOf(ctx, KEY).endsWith(CHUNK)).toBe(true);
  });

  it("shows the daemon's own sentence for a terminal.refused frame, and leaves the buffer alone", async () => {
    const { ctx } = await setup();
    applyTerminalFrame(ctx, screenFrame(0, "ready\r\n"), DAEMON_ID);
    applyTerminalFrame(
      ctx,
      {
        kind: "terminal.refused",
        frame: {
          type: "terminal.refused",
          cardId: DAEMON_ID,
          error: {
            code: "refused",
            message: "This card has no terminal running. Switch it to terminal view first.",
            details: { cardId: DAEMON_ID, reason: "terminal_not_active" },
          },
        },
      },
      DAEMON_ID,
    );
    expect(ctx.S.toasts.map((one) => one.msg)).toContain(
      "This card has no terminal running. Switch it to terminal view first.",
    );
    expect(terminalTextOf(ctx, KEY)).toBe("ready\r\n");
  });

  it("does nothing for a daemon id the store does not know", async () => {
    const { ctx } = await setup();
    applyTerminalOutputEvent(
      ctx,
      outputEvent(1, "for a card that does not exist", "not-a-real-id"),
    );
    applyTerminalFrame(ctx, screenFrame(0, "", "not-a-real-id"), "not-a-real-id");
    expect(terminalTextOf(ctx, KEY)).toBe("");
  });
});

/** Opens the card and waits for its own topic to actually be subscribed (`card-session.ts`'s
 * `followOpenCard`, already running since S7c/S8a/S10 default to the daemon): the fake's terminal
 * router checks a socket's latest hello before it answers, the same way the real one does. */
async function openAndFollow(ctx: Ctx, d: FakeDaemon): Promise<void> {
  ctx.S.openId = "api#41";
  const topic = `card:${cardId(41)}`;
  await vi.waitFor(() => {
    const hello = d.sockets
      .last()
      .hellos()
      .findLast((one) => one.type === "hello");
    expect(Array.isArray(hello?.subscribe) && hello.subscribe.includes(topic)).toBe(true);
  });
}

describe("sendTerminalText and sendTerminalKey", () => {
  it("sends typed text with a trailing line end, not trimmed", async () => {
    const { ctx, d } = await setup();
    await openAndFollow(ctx, d);
    sendTerminalText(ctx, "api#41", "ls -la");
    expect(d.terminalInputs(cardId(41))).toEqual([
      buildTerminalInput(cardId(41), { data: "ls -la\r" }),
    ]);
  });

  it("sends an empty field as a bare line end, the only way this view answers a bare Enter prompt", async () => {
    const { ctx, d } = await setup();
    await openAndFollow(ctx, d);
    sendTerminalText(ctx, "api#41", "");
    expect(d.terminalInputs(cardId(41))).toEqual([buildTerminalInput(cardId(41), { data: "\r" })]);
  });

  it("sends a named key", async () => {
    const { ctx, d } = await setup();
    await openAndFollow(ctx, d);
    sendTerminalKey(ctx, "api#41", "ctrl-c");
    expect(d.terminalInputs(cardId(41))).toEqual([
      buildTerminalInput(cardId(41), { key: "ctrl-c" }),
    ]);
  });

  it("does nothing for a card the store does not know", async () => {
    const { ctx, d } = await setup();
    await openAndFollow(ctx, d);
    sendTerminalText(ctx, "api#9999", "ls -la");
    sendTerminalKey(ctx, "api#9999", "esc");
    expect(d.terminalInputs(cardId(41))).toEqual([]);
  });
});
