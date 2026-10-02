import type { GoogleCalendarChoice } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { GoogleCalendarsAnswer } from "~/sync/google-actions";
import { GoogleCalendarPicker } from "./GoogleCalendarPicker";

afterEach(cleanup);

const calendar = (over: Partial<GoogleCalendarChoice>): GoogleCalendarChoice => ({
  id: "x",
  name: "X",
  color: "",
  primary: false,
  owned: true,
  selected: true,
  ...over,
});

const MINE = calendar({ id: "me@x.com", name: "me@x.com", primary: true });
const WORK = calendar({ id: "work", name: "Work" });
const HOLIDAYS = calendar({ id: "holidays", name: "Holidays", owned: false, selected: false });

const list = (chosen = false): GoogleCalendarsAnswer => ({
  choices: { calendars: [MINE, WORK, HOLIDAYS], chosen },
});

describe("the Google calendar tick list", () => {
  it("draws nothing until Google Calendar is connected, and asks Google nothing", () => {
    const load = vi.fn();
    render(() => <GoogleCalendarPicker connected={false} load={load} />);
    expect(screen.queryByText("Calendars Marshal reads")).toBeNull();
    expect(load).not.toHaveBeenCalled();
  });

  it("lists every calendar with the ones Marshal reads ticked, and marks a shared one", async () => {
    render(() => <GoogleCalendarPicker connected load={async () => list()} />);
    await screen.findByText("Work");
    expect(screen.getByLabelText(/me@x.com/)).toBeChecked();
    expect(screen.getByLabelText(/Work/)).toBeChecked();
    expect(screen.getByLabelText(/Holidays/)).not.toBeChecked();
    expect(screen.getByText("shared or subscribed")).toBeInTheDocument();
    expect(screen.getByText(/follows what is ticked in Google Calendar/)).toBeInTheDocument();
  });

  it("saves the whole ticked set when one is changed, and then says the choice is theirs", async () => {
    const save = vi.fn(
      async (ids: string[]): Promise<GoogleCalendarsAnswer> => ({
        choices: {
          chosen: true,
          calendars: [MINE, WORK, HOLIDAYS].map((one) => ({
            ...one,
            selected: ids.includes(one.id),
          })),
        },
      }),
    );
    render(() => <GoogleCalendarPicker connected load={async () => list()} save={save} />);
    await screen.findByText("Holidays");
    fireEvent.click(screen.getByLabelText(/Holidays/));
    await waitFor(() => expect(save).toHaveBeenCalledWith(["me@x.com", "work", "holidays"]));
    await screen.findByText("Marshal reads the calendars you ticked.");
    expect(screen.getByLabelText(/Holidays/)).toBeChecked();
  });

  it("says why the calendars could not be read, and tries again", async () => {
    const load = vi
      .fn<() => Promise<GoogleCalendarsAnswer>>()
      .mockResolvedValueOnce({ error: "Google Calendar is not connected yet." })
      .mockResolvedValueOnce(list());
    render(() => <GoogleCalendarPicker connected load={load} />);
    await screen.findByText("Google Calendar is not connected yet.");
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await screen.findByText("Work");
    expect(load).toHaveBeenCalledTimes(2);
  });

  it("shows the daemon's sentence when saving is refused, and keeps the ticks", async () => {
    const save = vi.fn(
      async (): Promise<GoogleCalendarsAnswer> => ({ error: "That calendar is not yours." }),
    );
    render(() => <GoogleCalendarPicker connected load={async () => list()} save={save} />);
    await screen.findByText("Holidays");
    fireEvent.click(screen.getByLabelText(/Holidays/));
    await screen.findByText("That calendar is not yours.");
  });
});
