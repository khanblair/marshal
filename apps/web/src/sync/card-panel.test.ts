import { afterEach, describe, expect, it, vi } from "vitest";
import { type SectionId, type SectionStatus, sectionStatus } from "~/data/sections";
import { wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { readPanel } from "./card-panel";

const CARD = wireCard({ projectId: "api", number: 41, title: "Fix token refresh on login" });
const KEY = CARD.key;
const ME = "01M3USER00000000000000000A";

const SECTIONS = {
  ...sectionStatus,
  S12: "daemon" as const,
  S15: "daemon" as const,
  S16: "daemon" as const,
};

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

async function store(sections: Readonly<Record<SectionId, SectionStatus>> = SECTIONS) {
  daemon = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, cards: [CARD] });
  const M = createTestMarshal({ data: daemon.data, sections });
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  const ctx = contextOf(M);
  const card = () => ctx.S.cards.find((one) => one.id === KEY)!;
  return { M, ctx, card, d: daemon };
}

describe("reading a card's panel", () => {
  it("puts the checks, checklists, comments, and members of the card in the store", async () => {
    const { ctx, d, card } = await store();
    const api = d.data.api;
    await api.createChecklist(CARD.id, { name: "Done when" });
    await api.postComment(CARD.id, { body: "hello", attachments: [] });
    await api.addCardMember(CARD.id, ME);

    await readPanel(ctx, api, CARD.id, KEY);

    expect(ctx.S.checks[KEY]?.map((c) => c.name)).toEqual([
      "Tests pass",
      "Lint clean",
      "Reviewer approval",
    ]);
    expect(card().checklists.map((l) => l.title)).toEqual(["Done when"]);
    expect(card().comments.map((c) => c.text)).toEqual(["hello"]);
    expect(card().comments[0]?.author).toBe(ME);
    expect(card().members).toEqual([ME]);
  });

  it("reads nothing for a section that is still the mock's", async () => {
    const { ctx, d, card } = await store({ ...SECTIONS, S15: "mock", S16: "mock", S12: "mock" });
    await d.data.api.createChecklist(CARD.id, { name: "Ignored" });
    await readPanel(ctx, d.data.api, CARD.id, KEY);
    expect(card().checklists).toEqual([]);
    expect(ctx.S.checks[KEY]?.[0]?.id).not.toMatch(/^01M3CHECK/);
  });
});

describe("checklists on the daemon", () => {
  it("adds a checklist and a line, ticks it as the person, hides done lines, and deletes it", async () => {
    const { M, ctx, d, card } = await store();
    await readPanel(ctx, d.data.api, CARD.id, KEY);

    M.addChecklist(KEY, "Release");
    await vi.waitFor(() => expect(card().checklists).toHaveLength(1));
    const list = () => card().checklists[0]!;
    expect(list().title).toBe("Release");

    M.addItem(KEY, list().id, "Write the notes");
    await vi.waitFor(() => expect(list().items).toHaveLength(1));

    M.toggleItem(KEY, list().id, list().items[0]!.id);
    await vi.waitFor(() => expect(list().items[0]?.done).toBe(true));
    expect(list().items[0]?.by).toBe(ME);
    expect(list().items[0]?.doneAt).toBeTypeOf("number");

    M.toggleHideDone(KEY, list().id);
    await vi.waitFor(() => expect(list().hideDone).toBe(true));

    M.deleteChecklist(KEY, list().id);
    expect(M.S.dialog?.action).toBe("Delete checklist");
    M.S.dialog?.run?.();
    await vi.waitFor(() => expect(card().checklists).toHaveLength(0));
  });

  it("shows the daemon's own sentence when it refuses, and leaves the list as it was", async () => {
    const { M, ctx, d, card } = await store();
    await readPanel(ctx, d.data.api, CARD.id, KEY);
    M.addChecklist(KEY, "One");
    await vi.waitFor(() => expect(card().checklists).toHaveLength(1));
    M.addItem(KEY, "01M3LIST0000000NOSUCHLIST", "x");
    await vi.waitFor(() => expect(M.S.toasts.at(-1)?.msg).toMatch(/cannot find that checklist/));
    expect(card().checklists[0]?.items).toEqual([]);
  });
});

describe("comments and members on the daemon", () => {
  it("posts a comment, turns a link in it into an attachment, and marks a question read", async () => {
    const { M, ctx, d, card } = await store();
    await readPanel(ctx, d.data.api, CARD.id, KEY);

    M.addComment(KEY, "https://example.com/spec is this the spec?");
    await vi.waitFor(() => expect(card().comments).toHaveLength(1));
    const comment = card().comments[0]!;
    expect(comment.att).toEqual([
      { kind: "link", name: "example.com/spec", url: "https://example.com/spec" },
    ]);
    expect(comment.read).toBe(true);

    M.addComment(KEY, "just a note");
    await vi.waitFor(() => expect(card().comments).toHaveLength(2));
    expect(card().comments[1]?.read).toBe(false);
  });

  it("deletes only the person's own comment", async () => {
    const { M, ctx, d, card } = await store();
    await readPanel(ctx, d.data.api, CARD.id, KEY);
    M.addComment(KEY, "mine");
    await vi.waitFor(() => expect(card().comments).toHaveLength(1));
    M.deleteComment(KEY, card().comments[0]!.id);
    await vi.waitFor(() => expect(card().comments).toHaveLength(0));
  });

  it("adds and removes a member", async () => {
    const { M, ctx, d, card } = await store();
    await readPanel(ctx, d.data.api, CARD.id, KEY);
    M.toggleMember(KEY, ME);
    await vi.waitFor(() => expect(card().members).toEqual([ME]));
    M.toggleMember(KEY, ME);
    await vi.waitFor(() => expect(card().members).toEqual([]));
  });

  it("reads a comment another screen posted when the daemon says one was created", async () => {
    const { ctx, d, card } = await store();
    await readPanel(ctx, d.data.api, CARD.id, KEY);
    await d.data.api.postComment(CARD.id, { body: "from the phone", attachments: [] });
    d.emit(`card:${CARD.id}`, "comment.created", { cardId: CARD.id });
    await vi.waitFor(() => expect(card().comments.map((c) => c.text)).toEqual(["from the phone"]));
    expect(ctx.S.cards).toBeDefined();
  });
});

describe("acceptance checks on the daemon", () => {
  it("shows the checks running, then what the daemon found", async () => {
    const { M, ctx, d } = await store();
    d.cardPanel.failing.add("Tests pass");
    await readPanel(ctx, d.data.api, CARD.id, KEY);
    M.runChecks(KEY);
    expect(ctx.S.checks[KEY]?.filter((c) => c.st === "running").map((c) => c.name)).toEqual([
      "Tests pass",
      "Lint clean",
    ]);
    await vi.waitFor(() => expect(ctx.S.checks[KEY]?.[0]?.st).toBe("failed"));
    expect(ctx.S.checks[KEY]?.[1]?.st).toBe("passed");
    expect(ctx.S.checks[KEY]?.[2]?.st).toBe("pending");
  });
});
