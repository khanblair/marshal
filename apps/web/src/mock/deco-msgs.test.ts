import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createTestMarshal, MOCK_HISTORY } from "~/testing/test-store";
import type { CardKey } from "./card-key";
import type { MsgView } from "./deco-msgs";
import type { Marshal } from "./marshal";

const make = (): Marshal =>
  createTestMarshal({
    hash: "#nosim",
    storage: null,
    viewport: { w: 1440, h: 900 },
    applyTheme: () => {},
    sections: MOCK_HISTORY,
  });

const views = (M: Marshal, id: CardKey): MsgView[] => M.decoMsgs(M.S.chat[id] ?? [], id);
const find = (M: Marshal, id: CardKey, pick: (v: MsgView) => boolean | undefined): MsgView => {
  const v = views(M, id).find(pick);
  if (!v) throw new Error("message not found");
  return v;
};

describe("decoMsgs handlers", () => {
  let M: Marshal;
  beforeEach(() => {
    vi.useFakeTimers();
    M = make();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it("splits tool actions into a verb and a target, mono for paths and commands", () => {
    const tools = views(M, "api#41").filter((v) => v.isTool);
    expect(tools[0]).toMatchObject({
      verb: "Read",
      target: "internal/auth/middleware.go",
      mono: true,
    });
    expect(tools.at(-1)).toMatchObject({
      stIcon: "spinner",
      resColor: "var(--color-text-secondary)",
    });
    const fail = find(M, "web#119", (v) => v.stIcon === "x");
    expect(fail).toMatchObject({ resColor: "var(--color-status-danger-text)", hasDetail: true });
    const commit = views(M, "api#41").find((v) => v.target?.startsWith('"'));
    expect(commit).toBeUndefined();
  });

  it("toggles a tool's details through the view model", () => {
    const tool = find(M, "api#41", (v) => v.hasDetail);
    expect(tool.chev).toBe("chevron-right");
    tool.toggle?.();
    expect(find(M, "api#41", (v) => v.hasDetail).chev).toBe("chevron-down");
  });

  it("opens the diff tab from the diff summary", () => {
    const diff = find(M, "api#41", (v) => v.isDiff);
    expect(diff).toMatchObject({ summary: "Changed 3 files", add: "+64", del: "−12" });
    diff.openDiff?.();
    expect(M.S).toMatchObject({ openId: "api#41", tab: "diff" });
    M.decoMsgs(M.S.chat["api#41"] ?? [], null)
      .find((v) => v.isDiff)
      ?.openDiff?.();
  });

  it("approves with Enter and denies with Escape on the approval", () => {
    const approval = find(M, "api#44", (v) => v.isApproval);
    const el = document.createElement("div");
    const escapeKey = new KeyboardEvent("keydown", { key: "Escape", cancelable: true });
    el.addEventListener("keydown", (e) => approval.keys?.(e));
    el.dispatchEvent(escapeKey);
    expect(escapeKey.defaultPrevented).toBe(true);
    expect(M.S.chat["api#44"]?.find((m) => m.k === "approval")).toMatchObject({ st: "denied" });
    const done = find(M, "api#44", (v) => v.isApproval);
    expect(done).toMatchObject({ done: true, resultLabel: "Denied", resultIcon: "x" });
    done.keys?.(new KeyboardEvent("keydown", { key: "Enter" }));
  });

  it("approves the project chat approval for its card", () => {
    const list = M.S.chats.api?.[1]?.msgs ?? [];
    const approval = M.decoMsgs(list, null).find((v) => v.isApproval);
    expect(approval).toMatchObject({ hasCard: true, cardLabel: "#44 Upgrade grpc-go to 1.66" });
    const el = document.createElement("div");
    el.addEventListener("keydown", (e) => approval?.keys?.(e));
    el.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    expect(M.card("api#44")?.state).toBe("working");
    approval?.openCard?.();
    expect(M.S.openId).toBe("api#44");
  });

  it("edits and saves a plan from its form", () => {
    const plan = find(M, "api#43", (v) => v.isPlan);
    expect(plan).toMatchObject({
      waiting: true,
      statusLabel: "Waiting for you",
      steps: expect.any(Array),
    });
    plan.edit?.();
    expect(find(M, "api#43", (v) => v.isPlan)).toMatchObject({ editing: true, notEditing: false });
    const form = document.createElement("form");
    const steps = document.createElement("textarea");
    steps.name = "steps";
    steps.value = "One\nTwo";
    form.append(steps);
    form.addEventListener("submit", (e) => plan.save?.(e));
    form.dispatchEvent(new SubmitEvent("submit", { cancelable: true }));
    expect(find(M, "api#43", (v) => v.isPlan)).toMatchObject({
      statusLabel: "Edited, waiting for you",
      editText: "One\nTwo",
    });
    plan.cancel?.();
    plan.reject?.();
    expect(find(M, "api#43", (v) => v.isPlan)).toMatchObject({
      done: true,
      statusLabel: "Rejected",
    });
  });

  it("approves a plan from the view model", () => {
    find(M, "api#43", (v) => v.isPlan).approve?.();
    expect(find(M, "api#43", (v) => v.isPlan)).toMatchObject({
      statusLabel: "Approved",
      statusIcon: "check",
    });
  });

  it("turns card links into card view models and hides missing cards", () => {
    const web = M.decoMsgs(M.S.chats.web?.[0]?.msgs ?? [], null);
    expect(web[1]?.cards?.map((c) => c.num)).toEqual(["#119", "#121"]);
    const missing = M.decoMsgs([{ id: "x", k: "card", cardId: "api#9999" }], null);
    expect(missing[0]?.isCard).toBe(false);
    const links = M.decoMsgs(
      [{ id: "y", k: "links", text: "t", cards: ["api#41", "api#9999"] }],
      null,
    );
    expect(links[0]?.cards).toHaveLength(1);
  });

  it("reads an empty field for forms without it", () => {
    const plan = find(M, "api#43", (v) => v.isPlan);
    const form = document.createElement("form");
    form.addEventListener("submit", (e) => plan.save?.(e));
    form.dispatchEvent(new SubmitEvent("submit", { cancelable: true }));
    expect(find(M, "api#43", (v) => v.isPlan)?.steps).toEqual([]);
    const div = document.createElement("div");
    div.addEventListener("submit", (e) => plan.save?.(e as SubmitEvent));
    div.dispatchEvent(new Event("submit"));
  });
});
