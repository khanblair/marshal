// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the notices are the daemon's).
import { daemon, resetNotices } from "~/testing/daemon-notices-store";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { NoticesPanel } from "./NoticesPanel";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * Section S23: the notices panel against the fake daemon. The daemon owns the notices, so the panel
 * draws the list it holds and every button asks the daemon; the list the screens then show arrives
 * from the daemon's own `notice.dismissed` event, not from the button. The fake daemon holds the
 * golden `notice-list`: one sleep group naming two cards, and the two cards it names.
 */

const NOTICE = "sleep:web-dashboard";
const ACTION = `POST /v1/notices/${encodeURIComponent(NOTICE)}/actions`;

const panel = (): HTMLElement => screen.getByRole("region", { name: "Notices" });
const articles = (): HTMLElement[] => within(panel()).queryAllByRole("article");
const sleepArticle = (): HTMLElement => {
  const found = articles().find((one) => one.textContent?.includes("idle and will sleep in"));
  if (!found) throw new Error("no sleep notice");
  return found;
};
const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);
const keepAwakeButtons = (): HTMLElement[] =>
  within(sleepArticle()).getAllByRole("button", { name: "Keep awake" });

beforeEach(() => {
  resetNotices();
  M.setViewport(1440, 900);
  M.go("home");
  // `go` closes the panel, so it is opened after the store is put back on Home.
  M.set({ noticesOpen: true, openId: null });
});
afterEach(cleanup);

describe("the notices panel on the daemon", () => {
  it("draws the daemon's own list, not the mock's seeded notices", () => {
    render(() => <NoticesPanel />);
    expect(articles()).toHaveLength(1);
    expect(sleepArticle()).toHaveTextContent(/2 cards are idle and will sleep in \d+:\d\d/);
    expect(keepAwakeButtons()).toHaveLength(2);
    // The sleep group names its cards, so it carries no Dismiss button of its own.
    expect(within(sleepArticle()).queryByRole("button", { name: "Dismiss notice" })).toBeNull();
  });

  it("keeps all awake: the daemon's own answer and the list its event brings back", async () => {
    render(() => <NoticesPanel />);
    fireEvent.click(within(sleepArticle()).getByRole("button", { name: "Keep all awake" }));
    await waitFor(() => expect(articles()).toHaveLength(0));
    expect(daemon.bodies(ACTION)).toEqual([{ action: "keep-all" }]);
    // The daemon's own event empties the list before the call's promise settles, so the sentence the
    // button says is waited for rather than read straight after the redraw.
    await waitFor(() => expect(toasts()).toEqual(["Kept 2 cards awake"]));
  });

  it("keeps one card awake, and the group the daemon sends back has one card left", async () => {
    render(() => <NoticesPanel />);
    fireEvent.click(keepAwakeButtons()[0] as HTMLElement);
    await waitFor(() => expect(keepAwakeButtons()).toHaveLength(1));
    expect(daemon.bodies(ACTION)).toEqual([
      { action: "keep-awake", cardId: "01JD7Q4M2X8K9V0P5T3RB6NHC3" },
    ]);
    expect(sleepArticle()).toHaveTextContent(/1 card is idle and will sleep in/);
    await waitFor(() => expect(toasts()).toEqual(["Kept awake for 15 more minutes"]));
  });

  it("sleeps them all now, and the group is gone", async () => {
    render(() => <NoticesPanel />);
    fireEvent.click(within(sleepArticle()).getByRole("button", { name: "Sleep all now" }));
    await waitFor(() => expect(articles()).toHaveLength(0));
    expect(daemon.bodies(ACTION)).toEqual([{ action: "sleep-all" }]);
    await waitFor(() => expect(toasts()).toEqual(["2 cards asleep"]));
  });

  it("shows the daemon's own sentence, and keeps the group, when it refuses", async () => {
    daemon.refuseNext(ACTION, 422, "refused", "Working cards don't sleep. Pause the card first.");
    render(() => <NoticesPanel />);
    fireEvent.click(within(sleepArticle()).getByRole("button", { name: "Sleep all now" }));
    await waitFor(() =>
      expect(toasts()).toEqual(["Working cards don't sleep. Pause the card first."]),
    );
    expect(keepAwakeButtons()).toHaveLength(2);
  });
});
