import { describe, expect, it, vi } from "vitest";
import type { Chat } from "~/mock";
import { M } from "~/mock";
import {
  composerPlaceholder,
  matchesQuery,
  newChatTargets,
  noChatsText,
  signatureChatId,
  targetBits,
  targetIcon,
  targetLabel,
  threadSignature,
} from "./chat-model";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const chat = (over: Partial<Chat> = {}): Chat => ({
  id: "ch1",
  pid: "api",
  title: "Plan the export",
  target: "Orchestrator",
  msgs: [],
  last: 0,
  archived: false,
  ...over,
});

describe("target labels", () => {
  it("names the target of a card, the Orchestrator, and a role", () => {
    const card = M.card("web#118");
    expect(targetLabel("web#118")).toBe(`#118 ${card?.agent}`);
    expect(targetLabel("api#99999")).toBe("#99999");
    expect(targetLabel("Orchestrator")).toBe("Orchestrator");
    expect(targetLabel("Tester")).toBe("Tester role");
  });

  it("picks an icon per kind of target", () => {
    expect(targetIcon("web#118")).toBe("bot");
    expect(targetIcon("Orchestrator")).toBe("route");
    expect(targetIcon("Reviewer")).toBe("user-cog");
  });

  it("lists the facts under the chat title", () => {
    const card = M.card("web#118");
    expect(targetBits(chat({ target: "web#118" }))).toEqual([
      card?.agent,
      card?.model,
      card?.asleep ? "Asleep" : "Awake",
    ]);
    expect(targetBits(chat())).toEqual(["Orchestrator", "claude-opus-4-1", "High thinking"]);
    expect(targetBits(chat({ target: "Tester" }))).toEqual([
      "Tester role",
      "Uses the role template",
    ]);
    expect(targetBits(chat({ target: "api#99999" }))).toEqual([
      "#99999 role",
      "Uses the role template",
    ]);
  });
});

describe("composerPlaceholder", () => {
  it("asks who the message goes to", () => {
    expect(composerPlaceholder(undefined)).toBe("Message");
    expect(composerPlaceholder(chat())).toBe("Message the Orchestrator");
    expect(composerPlaceholder(chat({ target: "Tester" }))).toBe("Message the Tester");
    expect(composerPlaceholder(chat({ target: "web#118" }))).toBe("Message #118");
  });
});

describe("matchesQuery", () => {
  const withMsgs = chat({
    msgs: [
      { id: "m1", k: "user", text: "Do we cache JWKS keys?" },
      { id: "m2", k: "card", cardId: "api#33" },
    ],
  });

  it("matches everything when the query is empty", () => {
    expect(matchesQuery(withMsgs, "")).toBe(true);
  });

  it("matches the title or any message text, ignoring case", () => {
    expect(matchesQuery(withMsgs, "EXPORT")).toBe(true);
    expect(matchesQuery(withMsgs, "jwks")).toBe(true);
    expect(matchesQuery(withMsgs, "nothing like this")).toBe(false);
  });
});

describe("noChatsText", () => {
  it("quotes the query as typed, or invites a first chat", () => {
    expect(noChatsText("Zzz")).toBe('No chats match "Zzz".');
    expect(noChatsText("")).toBe("No chats yet. Start one to plan work with the Orchestrator.");
  });
});

describe("newChatTargets", () => {
  it("lists the Orchestrator, the roles, then awake cards", () => {
    const targets = newChatTargets("api");
    expect(targets[0]).toEqual({ value: "Orchestrator", label: "Orchestrator" });
    expect(targets[1]).toEqual({ value: "Worker", label: "Worker role" });
    expect(targets.filter((t) => t.value === "Orchestrator")).toHaveLength(1);
    const awake = M.cardsOf("api").filter(M.isAwake);
    const cards = targets.filter((t) => t.value.includes("#"));
    expect(cards.map((t) => t.value)).toEqual(awake.map((c) => c.id));
    expect(cards[0]?.label).toBe(`#${awake[0]?.n} ${awake[0]?.title}`);
  });
});

describe("threadSignature", () => {
  it("changes with the chat, the message count, and the last message length", () => {
    expect(threadSignature(undefined)).toBe(":0:0");
    expect(threadSignature(chat())).toBe("ch1:0:0");
    const base = chat({ msgs: [{ id: "m1", k: "agent", text: "Hello" }] });
    expect(threadSignature(base)).toBe("ch1:1:5");
    const ref = chat({ msgs: [{ id: "m2", k: "card", cardId: "api#1" }] });
    expect(threadSignature(ref)).toBe("ch1:1:0");
    expect(signatureChatId(threadSignature(base))).toBe("ch1");
    expect(signatureChatId(":0:0")).toBe("");
  });
});
