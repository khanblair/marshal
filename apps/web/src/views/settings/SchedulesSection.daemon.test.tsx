// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the schedules are the daemon's).
import { daemon } from "~/testing/daemon-schedules-store";
import type { ScheduleList } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toScheduleRows } from "~/data/mappers/schedules";
import { golden } from "~/data/testing/golden";
import { M } from "~/mock";
import { createScheduleStore } from "~/testing/fake-schedules";
import { SettingsView } from "./SettingsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const POST = "POST /v1/schedules";
const MORNING_ID = "01H1234567890ABCDEFGHJKMNP";
const PUT_MORNING = `PUT /v1/schedules/${MORNING_ID}`;

type Body = Record<string, unknown>;
const last = (route: string): Body => {
  const sent = daemon.bodies(route);
  return sent[sent.length - 1] as Body;
};

/** The daemon and the store back at the golden list, with the Schedules section open. */
function reset(): void {
  Object.assign(daemon.schedules, createScheduleStore());
  daemon.calls.length = 0;
  const rows = toScheduleRows(golden<ScheduleList>("schedule-list"), (id) => id);
  M.set({ schedules: rows, settingsSection: "schedules", toasts: [] });
}

/** Opens the editor sheet of the Morning brief, once the screen is drawn. */
async function openMorningEditor(): Promise<void> {
  render(() => <SettingsView />);
  fireEvent.click(await screen.findByRole("button", { name: "Edit Morning brief" }));
  await screen.findByRole("button", { name: "Save schedule" });
}

beforeEach(reset);
afterEach(cleanup);

describe("New schedule", () => {
  it("offers the daemon's starter templates and a blank schedule", async () => {
    render(() => <SettingsView />);
    fireEvent.click(await screen.findByRole("button", { name: "New schedule" }));
    await screen.findByRole("menuitem", { name: "Morning brief" });
    const names = within(screen.getByRole("menu"))
      .getAllByRole("menuitem")
      .map((item) => item.textContent);
    expect(names).toEqual(["Morning brief", "Evening wind-down", "Stale cards", "Blank schedule"]);
  });

  it("makes a template switched off, with its parts and every chat, and opens its form", async () => {
    render(() => <SettingsView />);
    fireEvent.click(await screen.findByRole("button", { name: "New schedule" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Evening wind-down" }));
    await screen.findByRole("button", { name: "Save schedule" });
    expect(last(POST)).toMatchObject({
      name: "Evening wind-down",
      kind: "brief",
      enabled: false,
      project: "",
      template: "wind-down",
      sections: ["finished", "needs-you", "calendar"],
      deliver: ["telegram", "discord", "ntfy"],
    });
    expect(M.S.toasts.at(-1)?.msg).toContain("switched off");
  });

  it("makes a blank schedule a switched-off weekday brief, not a message to the Orchestrator", async () => {
    render(() => <SettingsView />);
    fireEvent.click(await screen.findByRole("button", { name: "New schedule" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Blank schedule" }));
    await screen.findByRole("button", { name: "Save schedule" });
    const sent = last(POST);
    expect(sent).toMatchObject({
      name: "New schedule",
      kind: "brief",
      trigger: "Cron",
      when: "Every weekday at 9:00",
      time: "09:00",
      enabled: false,
      template: "",
    });
    expect(String(sent.action)).not.toContain("Orchestrator");
    expect((sent.sections as string[]).length).toBeGreaterThan(0);
  });
});

describe("the schedule editor", () => {
  it("writes the words, the time, and the days together from the pickers", async () => {
    await openMorningEditor();
    fireEvent.input(screen.getByLabelText(/^Time \(/), { target: { value: "07:30" } });
    for (const day of ["Tue", "Wed", "Thu", "Fri"]) {
      fireEvent.click(screen.getByRole("button", { name: day }));
    }
    expect(screen.getByText("Every Monday at 7:30")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save schedule" }));
    await waitFor(() => expect(daemon.bodies(PUT_MORNING)).toHaveLength(1));
    expect(last(PUT_MORNING)).toMatchObject({
      when: "Every Monday at 7:30",
      time: "07:30",
      days: [1],
    });
  });

  it("will not leave a schedule with no day to run on", async () => {
    await openMorningEditor();
    for (const day of ["Mon", "Tue", "Wed", "Thu", "Fri"]) {
      fireEvent.click(screen.getByRole("button", { name: day }));
    }
    expect(screen.getByRole("button", { name: "Fri" })).toHaveAttribute("aria-pressed", "true");
  });

  it("lists what a brief can include and each chat with whether it is set up", async () => {
    await openMorningEditor();
    const include = screen.getByRole("group", { name: "What to include" });
    expect(within(include).getByRole("checkbox", { name: /Calendar/ })).toBeChecked();
    expect(within(include).getByRole("checkbox", { name: /Stale cards/ })).not.toBeChecked();
    const send = screen.getByRole("group", { name: "Send to" });
    expect(within(send).getByRole("checkbox", { name: /Telegram/ })).toBeChecked();
    expect(within(send).getAllByText("Not connected yet").length).toBeGreaterThan(0);
  });

  it("saves the parts and chats that were changed, and says them in the action", async () => {
    await openMorningEditor();
    fireEvent.click(
      within(screen.getByRole("group", { name: "What to include" })).getByRole("checkbox", {
        name: /Finished/,
      }),
    );
    fireEvent.click(
      within(screen.getByRole("group", { name: "Send to" })).getByRole("checkbox", {
        name: /Telegram/,
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Save schedule" }));
    await waitFor(() => expect(daemon.bodies(PUT_MORNING)).toHaveLength(1));
    const sent = last(PUT_MORNING);
    expect(sent.sections).toEqual(["calendar", "needs-you", "finished"]);
    expect(sent.deliver).toEqual([]);
    expect(String(sent.action)).toBe("Calendar, Needs you, Finished, kept in History");
  });

  it("gives an Event schedule words it can read, and refuses ones it cannot", async () => {
    await openMorningEditor();
    fireEvent.change(screen.getByRole("combobox", { name: "Trigger" }), {
      target: { value: "Event" },
    });
    const words = screen.getByLabelText("When, in words");
    expect(words).toHaveValue("When 30 minutes before my first calendar event");
    fireEvent.input(words, { target: { value: "Every weekday at 9:00" } });
    fireEvent.click(screen.getByRole("button", { name: "Save schedule" }));
    expect(await screen.findByText(/starts with "When"/)).toBeInTheDocument();
    expect(daemon.bodies(PUT_MORNING)).toHaveLength(0);
  });

  it("explains that a job cannot run yet", async () => {
    M.S.schedules.push({
      id: "job1",
      name: "Ask",
      kind: "job",
      icon: "clock",
      trigger: "Cron",
      when: "Every day at 9:00",
      time: "09:00",
      days: [],
      action: "Send a message to the Orchestrator",
      project: "All projects",
      projectId: "",
      enabled: false,
      missed: "Skip",
    });
    render(() => <SettingsView />);
    fireEvent.click(await screen.findByRole("button", { name: "Edit Ask" }));
    expect(await screen.findByText(/cannot run yet/)).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "What to include" })).toBeNull();
  });
});

describe("a schedule's row", () => {
  it("keeps its template, parts, chats, and quiet flag when it is switched on or off", async () => {
    render(() => <SettingsView />);
    fireEvent.click(
      await screen.findByRole("switch", { name: /Turn off Morning brief|Turn on Morning brief/ }),
    );
    await waitFor(() => expect(daemon.bodies(PUT_MORNING)).toHaveLength(1));
    expect(last(PUT_MORNING)).toMatchObject({
      template: "morning",
      sections: ["calendar", "needs-you"],
      deliver: ["telegram"],
      quietWhenEmpty: false,
    });
  });

  it("has its actions as icon buttons, with the delete last, on a line of their own", async () => {
    render(() => <SettingsView />);
    const row = (await screen.findByRole("button", { name: "Edit Morning brief" })).parentElement;
    const labels = within(row as HTMLElement)
      .getAllByRole("button")
      .map((button) => button.getAttribute("aria-label"));
    expect(labels).toEqual([
      "Preview Morning brief",
      "Run Morning brief now",
      "History of Morning brief",
      "Edit Morning brief",
      "Delete Morning brief",
    ]);
    expect(row).toHaveClass("justify-end", "col-span-3");
    expect(
      within(row as HTMLElement).getByRole("button", { name: "Delete Morning brief" }),
    ).toHaveClass("text-status-danger-text");
  });

  it("says where a brief goes", async () => {
    render(() => <SettingsView />);
    expect(await screen.findByText("Sends to Telegram")).toBeInTheDocument();
    expect(screen.getByText("Kept in History only")).toBeInTheDocument();
  });

  it("runs now and shows the run in the history sheet", async () => {
    render(() => <SettingsView />);
    fireEvent.click(await screen.findByRole("button", { name: "Run Morning brief now" }));
    await waitFor(() => expect(daemon.routes()).toContain(`POST /v1/schedules/${MORNING_ID}/run`));
    expect(await screen.findByText("Run by hand.")).toBeInTheDocument();
    expect(screen.getByRole("dialog")).toHaveTextContent("History");
    expect(M.S.toasts.at(-1)?.msg).toBe("Run finished");
  });

  it("shows a run's markdown as headings and a rule, not as pound signs", async () => {
    render(() => <SettingsView />);
    fireEvent.click(await screen.findByRole("button", { name: "Run Morning brief now" }));
    const sheet = await screen.findByRole("dialog");
    fireEvent.click(await within(sheet).findByRole("button", { expanded: false }));
    expect(await within(sheet).findByRole("heading", { name: "Finished" })).toBeInTheDocument();
    expect(
      within(sheet).getByRole("heading", { name: "Morning brief", level: 1 }),
    ).toBeInTheDocument();
    expect(sheet.querySelector("hr")).not.toBeNull();
    expect(sheet).not.toHaveTextContent("##");
  });

  it("folds each run under its time, and starts every one closed", async () => {
    render(() => <SettingsView />);
    const runNow = await screen.findByRole("button", { name: "Run Morning brief now" });
    fireEvent.click(runNow);
    fireEvent.click(
      await within(await screen.findByRole("dialog")).findByRole("button", { name: "Close" }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    fireEvent.click(runNow);
    const sheet = await screen.findByRole("dialog");
    const closed = await within(sheet).findAllByRole("button", { expanded: false });
    expect(closed).toHaveLength(2);
    expect(within(sheet).queryByRole("heading", { name: "Finished" })).toBeNull();
    expect(closed[0]).toHaveTextContent("Run by hand.");
    fireEvent.click(closed[0] as HTMLElement);
    expect(within(sheet).getAllByRole("heading", { name: "Finished" })).toHaveLength(1);
    fireEvent.click(closed[0] as HTMLElement);
    expect(within(sheet).queryByRole("heading", { name: "Finished" })).toBeNull();
  });

  it("asks before running a brief that goes to a chat that is set up", async () => {
    const row = M.S.integrations.find((one) => one.id === "telegram");
    if (!row) throw new Error("the store has no Telegram row");
    row.st = "connected";
    render(() => <SettingsView />);
    fireEvent.click(await screen.findByRole("button", { name: "Run Morning brief now" }));
    expect(daemon.routes()).not.toContain(`POST /v1/schedules/${MORNING_ID}/run`);
    expect(M.S.dialog?.message).toContain("Telegram");
    M.S.dialog?.run();
    await waitFor(() => expect(daemon.routes()).toContain(`POST /v1/schedules/${MORNING_ID}/run`));
    row.st = "none";
  });

  it("previews the message in a sheet, and sends nothing", async () => {
    render(() => <SettingsView />);
    fireEvent.click(await screen.findByRole("button", { name: "Preview Morning brief" }));
    const sheet = await screen.findByRole("dialog");
    expect(await within(sheet).findByText(/Needs you/)).toBeInTheDocument();
    expect(sheet).toHaveTextContent("Message preview");
    expect(daemon.routes()).not.toContain(`POST /v1/schedules/${MORNING_ID}/run`);
  });

  it("closes a sheet from its close button", async () => {
    render(() => <SettingsView />);
    fireEvent.click(await screen.findByRole("button", { name: "History of Morning brief" }));
    const sheet = await screen.findByRole("dialog");
    fireEvent.click(within(sheet).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("asks before deleting, from the icon", async () => {
    render(() => <SettingsView />);
    fireEvent.click(await screen.findByRole("button", { name: "Delete Morning brief" }));
    expect(M.S.dialog?.title).toBe('Delete "Morning brief"');
  });
});
