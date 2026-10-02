import type { GoogleCalendarChoice } from "@marshal/protocol";
import { Button, Checkbox } from "@marshal/ui";
import { createResource, createSignal, For, Show } from "solid-js";
import { M } from "~/mock";
import type { GoogleCalendarsAnswer } from "~/sync/google-actions";

const FALLBACK_DOT = "var(--color-border-strong)";

/** What the tick list is waiting on, or the sentence that says why it is not there. */
type Loaded =
  | { choices: { calendars: GoogleCalendarChoice[]; chosen: boolean } }
  | { error: string };

/**
 * The tick list of the person's Google calendars: Marshal reads the ticked ones for the calendar
 * view, Home's coming-up list, and the briefs. Until the person chooses, the ticks follow Google
 * Calendar's own side list; a calendar they were shared or subscribed to is marked as such.
 */
export function GoogleCalendarPicker(props: {
  connected: boolean;
  /** Reads the calendars. The store's own call by default; a test gives another. */
  load?: () => Promise<GoogleCalendarsAnswer>;
  /** Chooses the calendars. The store's own call by default; a test gives another. */
  save?: (ids: string[]) => Promise<GoogleCalendarsAnswer>;
}) {
  const [loaded, { mutate, refetch }] = createResource<Loaded | null, boolean>(
    () => props.connected,
    async (connected) => (connected ? (props.load ?? (() => M.googleCalendars()))() : null),
  );
  const [busy, setBusy] = createSignal(false);
  const [problem, setProblem] = createSignal("");

  const toggle = (calendars: GoogleCalendarChoice[], id: string, on: boolean): void => {
    const ids = calendars
      .filter((calendar) => (calendar.id === id ? on : calendar.selected))
      .map((calendar) => calendar.id);
    setBusy(true);
    setProblem("");
    void (props.save ?? ((chosen) => M.setGoogleCalendars(chosen)))(ids)
      .then((answer) => {
        if ("choices" in answer) mutate({ choices: answer.choices });
        else setProblem(answer.error);
      })
      .finally(() => setBusy(false));
  };

  return (
    <Show when={props.connected}>
      <div class="flex flex-col gap-2 border-t border-border pt-3">
        <span class="text-small font-semibold">Calendars Marshal reads</span>
        <Show when={loaded.loading}>
          <span class="text-small text-secondary">Reading your calendars…</span>
        </Show>
        <Show when={loaded()}>
          {(answer) => (
            <Show
              when={
                "choices" in answer()
                  ? (answer() as Extract<Loaded, { choices: unknown }>).choices
                  : null
              }
              fallback={
                <div class="flex flex-wrap items-center gap-2 text-small text-status-danger-text">
                  <span>{(answer() as { error: string }).error}</span>
                  <Button size={28} onClick={() => void refetch()}>
                    Try again
                  </Button>
                </div>
              }
            >
              {(choices) => (
                <>
                  <ul class="m-0 p-0 list-none flex flex-col gap-1">
                    <For each={choices().calendars}>
                      {(calendar) => (
                        <li>
                          <label class="flex items-center gap-2 text-small">
                            <Checkbox
                              checked={calendar.selected}
                              disabled={busy()}
                              onChange={(event) =>
                                toggle(
                                  choices().calendars,
                                  calendar.id,
                                  event.currentTarget.checked,
                                )
                              }
                            />
                            <span
                              aria-hidden="true"
                              class="flex-none size-2.5 rounded-full"
                              style={{ background: calendar.color || FALLBACK_DOT }}
                            />
                            <span class="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">
                              {calendar.name}
                            </span>
                            <Show when={!calendar.owned}>
                              <span class="text-caption text-muted">shared or subscribed</span>
                            </Show>
                          </label>
                        </li>
                      )}
                    </For>
                  </ul>
                  <span class="text-caption leading-4 text-secondary">
                    {choices().chosen
                      ? "Marshal reads the calendars you ticked."
                      : "Until you choose, Marshal follows what is ticked in Google Calendar."}
                  </span>
                </>
              )}
            </Show>
          )}
        </Show>
        <Show when={problem()}>
          <span class="text-small leading-4.5 text-status-danger-text">{problem()}</span>
        </Show>
      </div>
    </Show>
  );
}
