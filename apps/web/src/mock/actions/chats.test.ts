import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { applyProject } from "~/sync/projects";
import { daemonProject } from "~/testing/projects";
import { contextOf } from "~/testing/test-store";
import { FIXED_TIME, makeTwin, SLOW_TEST_MS, type Twin } from "../testing/twin";

const lastToast = (t: Twin): string | undefined => t.port.S.toasts.at(-1)?.msg;

describe("card messages", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it.each([
    ["api#41", "Also cover the logout path"],
    ["web#119", "Use createColumnHelper<ReportRow>()"],
    ["web#115", "Fix the tooltip on phones"],
    ["api#45", "Start with the admin routes"],
    ["api#33", "Anything left?"],
  ])("sends a message to %s and gets the scripted answer", (id, text) => {
    t.run("send", id, text);
    t.run("send", id, "   ");
    t.same();
    t.play(9000, 90);
  });
});

describe("project chats", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  const chatId = (pid: string, i: number): string => t.port.S.chats[pid]?.[i]?.id ?? "";

  it.each([
    ["web", "What is blocked?"],
    ["mobile", "Anything stuck?"],
    ["api", "Nothing waiting on me?"],
    ["api", "Make cards for the billing export"],
    ["api", "Create a task"],
    ["api", "Merge the next ready card"],
    ["web", "merge please"],
    ["api", "How is it going"],
  ])("answers %s: %s", (pid, text) => {
    t.run("chatSend", pid, chatId(pid, 0), text);
    t.same();
    t.play(6000, 90);
  });

  it("answers from a role target, and reports when nothing merges", () => {
    t.run("chatSend", "api", chatId("api", 2), "status?");
    t.play(6000, 200);
    t.removeProject("web");
    t.run("chatSend", "mobile", chatId("mobile", 0), "merge");
    t.play(6000, 200);
  });

  it("says nothing is blocked in a project without waiting cards", () => {
    // A project the daemon just sent has no cards, so this is the port alone (the prototype cannot add one this way).
    applyProject(contextOf(t.port), daemonProject({ id: "billing", path: "~/code/billing" }));
    const chat = t.port.newChat("billing", "");
    t.port.chatSend("billing", chat.id, "What is blocked?");
    vi.advanceTimersByTime(1000);
    expect(t.port.S.chats.billing?.[0]?.msgs.at(-1)).toMatchObject({
      text: "Nothing is blocked right now.",
    });
  });

  it("creates a chat that takes its name from the first message", () => {
    t.run("newChat", "api", "Tester");
    t.same();
    const chat = t.port.S.chats.api?.at(-1);
    expect(chat).toMatchObject({ title: "New chat", fresh: true, target: "Tester" });
    expect(t.port.S.chatOpen.api).toBe(chat?.id);
    t.run("chatSend", "api", chat?.id, "please look at the flaky tests now thanks!");
    t.same();
    expect(t.port.S.chats.api?.at(-1)?.title).toBe("Please look at the flaky tests");
    t.play(9000, 100);
  });

  it("returns the new chat as a live store object", () => {
    const chat = t.port.newChat("web");
    chat.title = "Renamed through the returned object";
    expect(t.port.chatById("web", chat.id)?.title).toBe("Renamed through the returned object");
  });

  it("rejects an empty chat name and keeps the old one", () => {
    const id = chatId("api", 0);
    expect(t.port.renameChat("api", id, "   ")).toBe(false);
    expect(lastToast(t)).toBe("Chat names can't be empty. The old name is kept.");
    expect(t.port.renameChat("api", id, "  Limits  ")).toBe(true);
    expect(t.port.chatById("api", id)?.title).toBe("Limits");
    t.proto.call("renameChat", "api", id, "   ");
    t.proto.call("renameChat", "api", id, "  Limits  ");
    t.same();
  });

  it("archives with an undo toast and restores", () => {
    const id = chatId("api", 0);
    t.run("openChat", "api", id);
    t.run("archiveChat", "api", id, true);
    t.same();
    expect(t.port.S.chatOpen.api).toBeNull();
    const undo = t.port.S.toasts.at(-1);
    expect(undo).toMatchObject({ msg: "Chat archived", action: { label: "Undo" } });
    undo?.action?.run();
    expect(t.port.chatById("api", id)?.archived).toBe(false);
    expect(lastToast(t)).toBe("Chat restored");
    t.run("archiveChat", "api", "nope", true);
  });

  it("deletes a chat after confirming", () => {
    const id = chatId("web", 0);
    t.run("openChat", "web", id);
    t.run("deleteChat", "web", id);
    expect(t.port.S.dialog?.message).toBe(
      'This deletes "What is blocked" and its messages. Cards it created stay on the board.',
    );
    t.confirmDialog();
    t.same();
    expect(t.port.S.chats.web).toHaveLength(1);
    expect(t.port.S.chatOpen.web).toBeNull();
  });

  it("lists chats newest first", () => {
    expect(t.ask("chatsOf", "api").port).toEqual(t.ask("chatsOf", "api").proto);
    expect(t.port.chatsOf("api").map((c) => c.title)[0]).toBe("Upgrade grpc-go");
    expect(t.port.chatsOf("nope")).toEqual([]);
  });
});

describe("projects", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it.each(["web", "mobile", "api"])("removes %s with its cards, chats, and notices", (pid) => {
    t.run("go", "project", pid);
    const first = t.port.cardsOf(pid)[0];
    t.run("openCard", first?.id);
    t.removeProject(pid);
    t.same();
    expect(t.port.cardsOf(pid)).toEqual([]);
    expect(t.port.S.openId).toBeNull();
    expect(t.port.S.route.page).toBe("home");
  });

  it("removes every project", () => {
    for (const pid of ["api", "web", "mobile"]) t.removeProject(pid);
    t.same();
    expect(t.port.S.route.pid).toBeNull();
  });
});
