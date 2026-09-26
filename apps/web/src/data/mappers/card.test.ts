import { type Label, SessionStateValues, type Card as WireCard } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "~/data/testing/golden";
import { DAY_MS } from "~/mock/constants";
import {
  agentKindOf,
  hasLiveSession,
  mirroredCard,
  permissionModeOf,
  sleepFlags,
  thinkingModeOf,
  toStoredCard,
} from "./card";

/** The golden card the Go tests wrote from the real wire type, as a card this test can change. */
const wireCard = (fields: Partial<WireCard> = {}): WireCard => ({
  ...structuredClone(golden<WireCard>("card")),
  ...fields,
});

/** A moment the same day as `from`, `days` away from it, so the day numbers do not depend on the zone. */
function dayFrom(from: number, days: number): string {
  const start = new Date(from);
  start.setHours(0, 0, 0, 0);
  return new Date(start.getTime() + days * DAY_MS).toISOString();
}

const NOW = Date.parse("2026-09-26T12:00:00.000Z");

/** One label of the golden card's project. */
const label = (id: string, name: string, color: Label["color"]): Label => ({
  id,
  projectId: "web-dashboard",
  name,
  color,
  createdAt: "2026-09-25T10:00:00.000Z",
});

describe("a card from the daemon", () => {
  it("is keyed by its key, because that is the name the screens and the routes use", () => {
    const card = toStoredCard(wireCard(), NOW);
    expect(card.id).toBe("web-dashboard#12");
    expect(card.n).toBe(12);
    expect(card.p).toBe("web-dashboard");
    expect(card.title).toBe("Add a health check endpoint");
    expect(card.state).toBe("backlog");
  });

  it("is given the words the screens show for the agent, the permission mode, and thinking", () => {
    const card = toStoredCard(wireCard(), NOW);
    expect(card.agent).toBe("Claude Code");
    expect(card.perm).toBe("Auto-accept edits");
    expect(card.think).toBe("High");
    expect(card.model).toBe("claude-sonnet-4-5");
  });

  it("carries the daemon's own words for what just happened, and names nothing the daemon does not send", () => {
    expect(toStoredCard(wireCard(), NOW)).toMatchObject({
      reason: "",
      doing: "",
      pr: null,
      ci: null,
    });
  });

  it("shows an empty branch and package as absent, not as an empty string", () => {
    expect(toStoredCard(wireCard(), NOW)).toMatchObject({ branch: null, pkg: null });
    const working = toStoredCard(
      wireCard({ branch: "marshal/api-41", package: "left-pad@2" }),
      NOW,
    );
    expect(working).toMatchObject({ branch: "marshal/api-41", pkg: "left-pad@2" });
  });

  it("has no thinking setting when the daemon sends none, and shows the two flags it does send", () => {
    expect(toStoredCard(wireCard({ thinking: null }), NOW).think).toBeNull();
    const card = toStoredCard(wireCard({ pinned: true, paused: true }), NOW);
    expect(card).toMatchObject({ pinned: true, paused: true });
  });

  it("shows the day numbers the Timeline draws, counted from the start of the daemon's today", () => {
    const card = toStoredCard(
      wireCard({
        plannedStart: dayFrom(NOW, -2),
        plannedEnd: dayFrom(NOW, 3),
        due: dayFrom(NOW, 0),
      }),
      NOW,
    );
    expect(card.s).toBe(-2);
    expect(card.e).toBe(3);
    expect(card.due).toBe(0);
  });

  it("has no dates when the daemon sends none, rather than day zero", () => {
    expect(toStoredCard(wireCard(), NOW)).toMatchObject({ s: null, e: null, due: null });
  });

  it("carries the card's labels by name, its context use as a fraction, and the moment it changed", () => {
    const card = toStoredCard(
      wireCard({
        labels: [
          label("01M3C107JB041061050R3GG28B", "auth", "blue"),
          label("01M3C107JB041061050R3GG28C", "bug", "red"),
        ],
        contextUsed: 46,
        updatedAt: "2026-09-26T09:30:00.000Z",
      }),
      NOW,
    );
    expect(card.labels).toEqual(["auth", "bug"]);
    expect(card.ctx).toBeCloseTo(0.46);
    expect(card.upd).toBe(Date.parse("2026-09-26T09:30:00.000Z"));
  });

  it("does not share a list with the wire object, so a later change to the answer cannot reach the store", () => {
    const wire = wireCard({
      labels: [label("01M3C107JB041061050R3GG28B", "auth", "blue")],
    });
    const card = toStoredCard(wire, NOW);
    wire.labels.push(label("01M3C107JB041061050R3GG28D", "bug", "red"));
    expect(card.labels).toEqual(["auth"]);
  });
});

describe("a card's session (section S7c)", () => {
  /** What each stored state means for the card: the state, the two flags, and whether an agent runs. */
  const MEANING = [
    ["starting", false, false, true],
    ["awake", false, false, true],
    ["working", false, false, true],
    ["waiting-approval", false, false, true],
    ["sleep-warning", false, false, true],
    ["asleep", true, false, false],
    ["waking", true, true, false],
    ["stopped", false, false, false],
  ] as const;

  it("says something about every state the daemon can send", () => {
    expect(MEANING.map(([state]) => state).sort()).toEqual([...SessionStateValues].sort());
  });

  it.each(MEANING)(
    "reads %s as asleep %s, waking %s, and a running agent %s",
    (state, asleep, waking, live) => {
      const card = toStoredCard(wireCard({ session: state }), NOW);
      expect(card).toMatchObject({ session: state, asleep, waking });
      expect(mirroredCard(wireCard({ session: state }), NOW)).toMatchObject({
        session: state,
        asleep,
        waking,
      });
      expect(hasLiveSession(state)).toBe(live);
      expect(sleepFlags(state)).toEqual({ asleep, waking });
    },
  );

  it("reads a card that never had a session as neither asleep nor waking, with no running agent", () => {
    expect(toStoredCard(wireCard({ session: null }), NOW)).toMatchObject({
      session: null,
      asleep: false,
      waking: false,
    });
    expect(hasLiveSession(null)).toBe(false);
    expect(hasLiveSession(undefined)).toBe(false);
    expect(sleepFlags(null)).toEqual({ asleep: false, waking: false });
  });

  it("reads an answer with no session at all, from a daemon that does not send one, as no session", () => {
    const wire = wireCard();
    Reflect.deleteProperty(wire, "session");
    expect(toStoredCard(wire, NOW).session).toBeNull();
  });
});

describe("a card's view (section S9)", () => {
  it("carries which view the card opens in, chat or terminal", () => {
    expect(toStoredCard(wireCard({ viewMode: "chat" }), NOW).viewMode).toBe("chat");
    expect(toStoredCard(wireCard({ viewMode: "terminal" }), NOW).viewMode).toBe("terminal");
    expect(mirroredCard(wireCard({ viewMode: "terminal" }), NOW)).toMatchObject({
      viewMode: "terminal",
    });
  });
});

describe("updating a card that is already in the store", () => {
  it("leaves out the three fields that name the card, which never change", () => {
    const mirrored = mirroredCard(wireCard(), NOW);
    expect(Object.keys(mirrored)).not.toContain("id");
    expect(Object.keys(mirrored)).not.toContain("p");
    expect(Object.keys(mirrored)).not.toContain("n");
    expect(mirrored).toMatchObject({ title: "Add a health check endpoint", state: "backlog" });
  });

  it("leaves out nothing else: every field it does carry is one the daemon owns", () => {
    const { id: _id, p: _p, n: _n, ...rest } = toStoredCard(wireCard(), NOW);
    expect(Object.keys(mirroredCard(wireCard(), NOW)).sort()).toEqual(Object.keys(rest).sort());
  });
});

describe("the words the screens use, back to what the daemon wants", () => {
  it("turns each name the screens show into the daemon's own value", () => {
    expect(agentKindOf("Claude Code")).toBe("claude");
    expect(agentKindOf("Gemini CLI")).toBe("gemini");
    expect(agentKindOf("Codex")).toBe("codex");
    expect(agentKindOf("Built-in agent")).toBe("builtin");
    expect(permissionModeOf("Auto-accept edits")).toBe("auto-edits");
    expect(permissionModeOf("Bypass permissions")).toBe("bypass");
    expect(thinkingModeOf("Extra high")).toBe("extra-high");
  });

  it("round-trips every value the daemon can send", () => {
    for (const name of ["Claude Code", "Gemini CLI", "Codex", "Built-in agent"]) {
      expect(agentKindOf(toStoredCard(wireCard(), NOW).agent)).toBeDefined();
      expect(agentKindOf(name)).toBeDefined();
    }
    for (const name of [
      "Ask",
      "Auto-accept edits",
      "Plan only",
      "Full auto",
      "Bypass permissions",
    ]) {
      expect(permissionModeOf(name)).toBeDefined();
      expect(
        permissionModeOf(
          toStoredCard(wireCard({ permissionMode: permissionModeOf(name)! }), NOW).perm,
        ),
      ).toBe(permissionModeOf(name));
    }
    for (const name of ["Low", "Medium", "High", "Extra high"]) {
      expect(
        thinkingModeOf(toStoredCard(wireCard({ thinking: thinkingModeOf(name)! }), NOW).think!),
      ).toBe(thinkingModeOf(name));
    }
  });

  it("answers nothing for a name Marshal does not know, rather than guessing", () => {
    expect(agentKindOf("Some other CLI")).toBeUndefined();
    expect(permissionModeOf("Yolo")).toBeUndefined();
    expect(thinkingModeOf("Very high")).toBeUndefined();
  });

  it("shows the daemon's own word when the daemon sends a value the screens do not know", () => {
    const card = toStoredCard(
      wireCard({ agent: "future-cli" as WireCard["agent"], permissionMode: "future" as never }),
      NOW,
    );
    expect(card.agent).toBe("future-cli");
    expect(card.perm).toBe("future");
  });
});
