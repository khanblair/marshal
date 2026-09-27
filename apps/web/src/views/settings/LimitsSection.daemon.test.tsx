// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the cost numbers and the limits are the daemon's).
import { ctx, daemon, resetLimits } from "~/testing/daemon-limits-store";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { applyLimitList } from "~/sync/limits";
import { SettingsView } from "./SettingsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * Section S26b: the cost and awake limits form against the fake daemon. The daemon answers with the
 * golden `limit-list`, so the "All projects" row shows $25 a day, $400 a month, and 18 awake cards,
 * and every project the screens draw has no ceiling of its own (the golden list's own project scope
 * names an id this store has no project for). Each save is one PUT or DELETE per field that changed,
 * in micro-dollars for a cost and as a count for awake.
 */

const DAY = "Daily cost limit";
const MONTH = "Monthly cost limit";
const AWAKE = "Awake card limit";
const SAVE = "Save limits";
const DAILY_LIMIT = "PUT /v1/limits/global/cost-day";

beforeEach(() => {
  resetLimits();
  // The store mirrors the daemon again, and the screen starts on the limits page with nothing said.
  applyLimitList(ctx, { limits: daemon.limits.limits });
  M.set({ settingsSection: "limits", toasts: [], dialog: null });
});
afterEach(cleanup);

const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);
// Each field's label also holds the `$` sign and the help line under it, so the name is matched
// loosely; the fields come in document order, the all-projects row first and then the projects.
const fields = (label: string): HTMLInputElement[] =>
  screen.getAllByLabelText<HTMLInputElement>(label, { exact: false });
const globalField = (label: string): HTMLInputElement => {
  const [first] = fields(label);
  if (!first) throw new Error(`no ${label} field`);
  return first;
};
const save = (): void => {
  fireEvent.click(screen.getByRole("button", { name: SAVE }));
};

describe("the cost and awake limits on the daemon", () => {
  it("shows the daemon's own ceilings, and an empty field for a scope that has none", () => {
    render(() => <SettingsView />);
    expect(globalField(DAY)).toHaveValue(25);
    expect(globalField(MONTH)).toHaveValue(400);
    expect(globalField(AWAKE)).toHaveValue(18);
    // The all-projects row first, then the store's three projects: none of theirs has a ceiling.
    expect(fields(DAY)).toHaveLength(4);
    expect(fields(DAY)[1]?.value).toBe("");
    expect(screen.getAllByText("Used $0.00. No limit set.")).not.toHaveLength(0);
  });

  it("saves a lowered ceiling with one PUT, in micro-dollars, and keeps the daemon's answer", async () => {
    render(() => <SettingsView />);
    fireEvent.input(globalField(DAY), { target: { value: "20" } });
    save();
    await waitFor(() => expect(toasts()).toContain("Limits saved"));
    expect(daemon.bodies(DAILY_LIMIT).at(-1)).toEqual({ value: 20_000_000 });
    // Only the one field was written: the month and the awake ceiling are untouched.
    expect(daemon.routes().filter((route) => route.startsWith("PUT /v1/limits"))).toEqual([
      DAILY_LIMIT,
    ]);
    expect(globalField(DAY)).toHaveValue(20);
  });

  it("removes a ceiling with a DELETE when the field is cleared", async () => {
    render(() => <SettingsView />);
    fireEvent.input(globalField(DAY), { target: { value: "" } });
    save();
    await waitFor(() => expect(toasts()).toContain("Limits saved"));
    expect(daemon.routes()).toContain("DELETE /v1/limits/global/cost-day");
    expect(daemon.bodies(DAILY_LIMIT)).toEqual([]);
    // The daemon's answer no longer has that ceiling, so the field stays empty and says so.
    expect(globalField(DAY).value).toBe("");
    expect(ctx.S.limits.global.day).toBeUndefined();
  });

  it("sends a raised awake ceiling as a count, with no cents", async () => {
    render(() => <SettingsView />);
    fireEvent.input(globalField(AWAKE), { target: { value: "24" } });
    save();
    await waitFor(() => expect(toasts()).toContain("Limits saved"));
    expect(daemon.bodies("PUT /v1/limits/global/awake").at(-1)).toEqual({ value: 24 });
  });

  it("refuses a typed zero itself, without calling the daemon", () => {
    render(() => <SettingsView />);
    fireEvent.input(globalField(DAY), { target: { value: "0" } });
    save();
    expect(toasts()).toContain("Fix the limits marked in red before saving.");
    expect(daemon.routes()).not.toContain(DAILY_LIMIT);
    expect(screen.getAllByText("Enter a limit above zero.")).toHaveLength(1);
  });

  it("asks before raising a cost ceiling, and only writes it once it is confirmed", async () => {
    render(() => <SettingsView />);
    fireEvent.input(globalField(DAY), { target: { value: "30" } });
    save();
    const dialog = M.S.dialog;
    expect(dialog?.title).toBe("Raise cost limit");
    expect(dialog?.message).toBe(
      "Agents can spend more before Marshal pauses them in all projects.",
    );
    expect(daemon.routes()).not.toContain(DAILY_LIMIT);
    dialog?.run();
    await waitFor(() => expect(toasts()).toContain("Limits saved"));
    expect(daemon.bodies(DAILY_LIMIT).at(-1)).toEqual({ value: 30_000_000 });
  });

  it("shows the daemon's own sentence when it refuses, and keeps the edit", async () => {
    daemon.refuseNext(DAILY_LIMIT, 400, "invalid_argument", "A limit must be more than zero.");
    render(() => <SettingsView />);
    fireEvent.input(globalField(DAY), { target: { value: "20" } });
    save();
    await waitFor(() => expect(toasts()).toContain("A limit must be more than zero."));
    // The store keeps the ceiling the daemon still has; the form keeps what was typed.
    expect(ctx.S.limits.global.day).toBe(25);
    expect(globalField(DAY)).toHaveValue(20);
  });

  it("counts the spend the daemon stored for today against the ceiling", () => {
    // The cost numbers are the daemon's once S19b is switched, so the help line reads `S.stats`.
    M.S.stats = {
      range: 7,
      days: [
        { day: ctx.today, cardsFinished: 0, merges: 0, ciFailures: 0, costMicros: 23_000_000 },
      ],
      projects: [],
    };
    render(() => <SettingsView />);
    expect(screen.getAllByText("Used $23.00. Near this limit.")).toHaveLength(1);
  });
});
