// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (S5a and S7c are the daemon's).
import { daemon, resetDaemonCards, resetStoreCards } from "~/testing/daemon-cards-store";
import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useGlobalKeys } from "~/app/keyboard";
import { resetShell } from "~/app/shell-test-utils";
import { type Card, M } from "~/mock";
import type { CardKey } from "~/mock/card-key";
import { actionsFor } from "../agents/agent-actions";
import { AwakeSection } from "../home/AwakeSection";
import { cardActions } from "./card-actions";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * The controls a person presses (section S7c): the card panel's buttons, the S and P keys, Home's
 * awake list, and the Agents view's actions, all against the fake daemon that answers as the real one
 * does (its refusal sentences, and the `card.updated` it publishes after every change). The cards
 * are the prototype's 29 as the daemon's fixture seeds them: api#41 works with a session that is
 * working, and api#39 is in review with one that is awake.
 */

const WORKING: CardKey = "api#41";
const REVIEW: CardKey = "api#39";
const NEEDS: CardKey = "api#43";

beforeEach(() => {
  resetDaemonCards();
  resetStoreCards();
  // Onboarding and the tour are off, every layer is closed, and no card is open or focused.
  resetShell();
  daemon.calls.splice(0, daemon.calls.length);
});
afterEach(cleanup);

const card = (key: CardKey): Card => {
  const one = M.card(key);
  if (!one) throw new Error(`no card ${key}`);
  return one;
};
const daemonId = (key: CardKey): string => encodeURIComponent(card(key).daemonId ?? "");
const route = (key: CardKey, action: string): string => `POST /v1/cards/${daemonId(key)}/${action}`;
const labels = (key: CardKey): string[] => cardActions(card(key), false).map((a) => a.label);
const press = (key: CardKey, label: string): void => {
  const action = cardActions(card(key), false).find((a) => a.label === label);
  if (!action) throw new Error(`${key} has no ${label} button: ${labels(key).join(", ")}`);
  action.run();
};
const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);
const wireOf = (key: CardKey) => {
  const wire = daemon.cards.find((one) => one.key === key);
  if (!wire) throw new Error(`the daemon has no ${key}`);
  return wire;
};

describe("the card panel's buttons", () => {
  it("pauses a working card, and Resume card releases the pause", async () => {
    expect(labels(WORKING)).toEqual(["Pause", "Sleep", "Pin", "Fork"]);
    press(WORKING, "Pause");
    await vi.waitFor(() => expect(card(WORKING).paused).toBe(true));
    expect(daemon.routes()).toContain(route(WORKING, "pause"));
    expect(toasts()).toEqual(["Card paused"]);
    expect(labels(WORKING)[0]).toBe("Resume card");

    press(WORKING, "Resume card");
    await vi.waitFor(() => expect(card(WORKING).paused).toBe(false));
    expect(daemon.routes()).toContain(route(WORKING, "start"));
    expect(toasts()).toContain("Card resumed");
    expect(labels(WORKING)[0]).toBe("Pause");
  });

  it("refuses to put a working card to sleep, with the daemon's sentence, and changes nothing", async () => {
    press(WORKING, "Sleep");
    await vi.waitFor(() =>
      expect(toasts()).toEqual(["Working cards don't sleep. Pause the card first."]),
    );
    expect(card(WORKING)).toMatchObject({ asleep: false, paused: false, session: "working" });
  });

  it("puts a paused card to sleep, shows Resume session, and wakes it into work with its context", async () => {
    press(WORKING, "Pause");
    await vi.waitFor(() => expect(card(WORKING).paused).toBe(true));
    press(WORKING, "Sleep");
    await vi.waitFor(() =>
      expect(card(WORKING)).toMatchObject({ asleep: true, session: "asleep" }),
    );
    expect(toasts()).toEqual(["Card paused", "Card asleep"]);
    // A sleeping card offers Resume session, and no second Sleep.
    expect(labels(WORKING)).toEqual(["Resume session", "Pin", "Fork"]);
    expect(cardActions(card(WORKING), false)[0]).toMatchObject({ primary: true, disabled: false });

    press(WORKING, "Resume session");
    await vi.waitFor(() =>
      expect(card(WORKING)).toMatchObject({ asleep: false, waking: false, state: "working" }),
    );
    expect(daemon.routes()).toContain(route(WORKING, "wake"));
    expect(toasts().at(-1)).toBe("Session resumed");
  });

  it("disables the button while the daemon says the session is waking", async () => {
    press(WORKING, "Pause");
    await vi.waitFor(() => expect(card(WORKING).paused).toBe(true));
    const wire = wireOf(WORKING);
    wire.session = "waking";
    daemon.emit("project:api", "card.updated", { card: wire });
    await vi.waitFor(() => expect(card(WORKING)).toMatchObject({ asleep: true, waking: true }));
    expect(cardActions(card(WORKING), false)[0]).toMatchObject({ label: "Waking", disabled: true });
  });

  it("refuses to sleep a card that is waiting on the person, in the daemon's words", async () => {
    press(NEEDS, "Sleep");
    await vi.waitFor(() =>
      expect(toasts()).toEqual(["This card is waiting on you, so it stays awake."]),
    );
    expect(card(NEEDS).asleep).toBe(false);
  });

  it("refuses to sleep a card whose session is not awake", async () => {
    const wire = wireOf(REVIEW);
    wire.session = "stopped";
    daemon.emit("project:api", "card.updated", { card: wire });
    await vi.waitFor(() => expect(card(REVIEW).session).toBe("stopped"));
    press(REVIEW, "Sleep");
    await vi.waitFor(() => expect(toasts()).toEqual(["This card has no awake session."]));
    expect(card(REVIEW).asleep).toBe(false);
  });

  it("pins and unpins, from the card the daemon answers with", async () => {
    press(NEEDS, "Pin");
    await vi.waitFor(() => expect(card(NEEDS).pinned).toBe(true));
    expect(toasts()).toEqual(["Card pinned. It won't sleep."]);
    expect(labels(NEEDS)).toContain("Unpin");
    press(NEEDS, "Unpin");
    await vi.waitFor(() => expect(card(NEEDS).pinned).toBe(false));
    expect(toasts()).toContain("Card unpinned");
    expect(daemon.routes()).toContain(route(NEEDS, "unpin"));
  });

  it("says Marshal is not connected when the daemon is away, and changes nothing", async () => {
    daemon.stop();
    try {
      press(REVIEW, "Sleep");
      await vi.waitFor(() => expect(toasts().length).toBeGreaterThan(0));
      expect(card(REVIEW)).toMatchObject({ asleep: false, session: "awake" });
    } finally {
      daemon.start();
    }
  });
});

describe("the S and P keys", () => {
  function Keys() {
    useGlobalKeys();
    return <div />;
  }
  const press1 = (key: string) => fireEvent.keyDown(window, { key });

  it("S puts the open card to sleep and wakes it again, and P pins it", async () => {
    render(() => <Keys />);
    M.S.openId = REVIEW;
    press1("s");
    await vi.waitFor(() => expect(card(REVIEW).asleep).toBe(true));
    expect(daemon.routes()).toContain(route(REVIEW, "sleep"));
    press1("s");
    await vi.waitFor(() => expect(card(REVIEW)).toMatchObject({ asleep: false, session: "awake" }));
    expect(daemon.routes()).toContain(route(REVIEW, "wake"));
    press1("p");
    await vi.waitFor(() => expect(card(REVIEW).pinned).toBe(true));
    press1("p");
    await vi.waitFor(() => expect(card(REVIEW).pinned).toBe(false));
  });

  it("S shows the daemon's sentence for a card that may not sleep", async () => {
    render(() => <Keys />);
    M.S.openId = WORKING;
    press1("s");
    await vi.waitFor(() =>
      expect(toasts()).toEqual(["Working cards don't sleep. Pause the card first."]),
    );
  });
});

describe("Home's awake list", () => {
  const section = () => screen.getByRole("region", { name: "Agents awake" });
  const sleepButton = (key: CardKey) =>
    within(section()).queryByRole("button", { name: `Sleep ${M.cardLabelOf(card(key))}` });

  it("lists the cards whose session has an agent, and hears a sleep without the card being open", async () => {
    render(() => <AwakeSection />);
    // The list shows five rows until it is asked for the rest, and a card in review comes late.
    fireEvent.click(within(section()).getByRole("button", { name: /^Show all/ }));
    expect(sleepButton(REVIEW)).toBeInTheDocument();
    const before = M.awake().length;
    expect(screen.getByText(`${before} of ${M.S.limits.global.awake}`)).toBeInTheDocument();
    expect(M.S.openId).toBeNull();
    fireEvent.click(sleepButton(REVIEW) as HTMLElement);
    await vi.waitFor(() => expect(card(REVIEW).asleep).toBe(true));
    expect(daemon.routes()).toContain(route(REVIEW, "sleep"));
    await vi.waitFor(() => expect(sleepButton(REVIEW)).not.toBeInTheDocument());
    expect(screen.getByText(`${before - 1} of ${M.S.limits.global.awake}`)).toBeInTheDocument();

    // A wake, from anywhere, puts it back in the list.
    press(REVIEW, "Resume session");
    await vi.waitFor(() => expect(sleepButton(REVIEW)).toBeInTheDocument());
    expect(M.awake()).toHaveLength(before);
  });

  it("shows the daemon's sentence when a Sleep is refused, and keeps the card in the list", async () => {
    render(() => <AwakeSection />);
    fireEvent.click(sleepButton(WORKING) as HTMLElement);
    await vi.waitFor(() =>
      expect(toasts()).toEqual(["Working cards don't sleep. Pause the card first."]),
    );
    expect(sleepButton(WORKING)).toBeInTheDocument();
  });

  it("leaves a card out that is in review with no session, which the column alone used to count", async () => {
    const wire = wireOf(REVIEW);
    wire.session = null;
    daemon.emit("project:api", "card.updated", { card: wire });
    render(() => <AwakeSection />);
    await vi.waitFor(() => expect(card(REVIEW).session).toBeNull());
    expect(sleepButton(REVIEW)).not.toBeInTheDocument();
  });
});

describe("the Agents view's actions", () => {
  const run = (key: CardKey, label: string): void => {
    const action = actionsFor(card(key)).find((a) => a.label === label);
    if (!action)
      throw new Error(`${key} has no ${label}: ${actionsFor(card(key)).map((a) => a.label)}`);
    action.run();
  };

  it("sleeps and wakes a card, and pins and unpins it", async () => {
    run(REVIEW, "Sleep");
    await vi.waitFor(() => expect(card(REVIEW).asleep).toBe(true));
    expect(actionsFor(card(REVIEW)).map((a) => a.label)).toEqual(["Open", "Wake", "Pin"]);
    run(REVIEW, "Wake");
    await vi.waitFor(() => expect(card(REVIEW)).toMatchObject({ asleep: false, session: "awake" }));
    run(REVIEW, "Pin");
    await vi.waitFor(() => expect(card(REVIEW).pinned).toBe(true));
    run(REVIEW, "Unpin");
    await vi.waitFor(() => expect(card(REVIEW).pinned).toBe(false));
  });

  it("asks first, and Stop then holds a working card and puts it to sleep", async () => {
    run(WORKING, "Stop");
    expect(M.S.dialog).toMatchObject({ title: "Stop session", action: "Stop session" });
    expect(card(WORKING).asleep).toBe(false);
    M.S.dialog?.run();
    await vi.waitFor(() => expect(card(WORKING)).toMatchObject({ asleep: true, paused: true }));
    expect(daemon.routes().filter((one) => /\/(pause|sleep)$/.test(one))).toEqual([
      route(WORKING, "pause"),
      route(WORKING, "sleep"),
    ]);
    expect(toasts()).toEqual(["Session stopped"]);
  });

  it("shows the daemon's sentence when Stop is refused, and leaves the card as it was", async () => {
    run(NEEDS, "Stop");
    M.S.dialog?.run();
    await vi.waitFor(() =>
      expect(toasts()).toEqual(["This card is waiting on you, so it stays awake."]),
    );
    expect(card(NEEDS)).toMatchObject({ asleep: false, paused: false });
  });
});
