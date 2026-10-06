import type { ScheduleCatalog } from "@marshal/protocol";
import { Button, Checkbox, cx, Field, Input, Select } from "@marshal/ui";
import { For, Show, untrack } from "solid-js";
import { M, type Schedule } from "~/mock";
import { GRID_MIN_200 } from "./auto-fit-grid";
import type { EditState } from "./edit-state";
import {
  createScheduleForm,
  MISSED_RUNS,
  type ScheduleForm,
  TRIGGERS,
  WORDS,
} from "./schedule-form";
import { DAYS, normalDays, orderedSections, toggled, WEEKDAYS, WEEKEND } from "./schedule-words";

const CHIP =
  "inline-flex items-center justify-center min-w-11 h-7 phone:h-(--control-h) px-2.5 rounded-sm border text-small font-medium";
const CHIP_ON = "border-ink bg-ink text-on-ink";
const CHIP_OFF = "border-border-strong bg-surface text-primary hover:bg-surface-hover";

const connected = (channel: string): boolean =>
  M.S.integrations.find((row) => row.id === channel)?.st === "connected";

/** The days of the week as buttons that stay pressed, and the three usual choices under them. */
function DayPicker(props: { form: ScheduleForm }) {
  const every = DAYS.map((day) => day.value);
  const on = (day: number) => props.form.days().length === 0 || props.form.days().includes(day);
  const flip = (day: number) => {
    const current = props.form.days().length === 0 ? every : props.form.days();
    const next = current.includes(day) ? current.filter((one) => one !== day) : [...current, day];
    // A schedule that ran on no day would never run, so the last day stays.
    if (next.length > 0) props.form.setDays(normalDays(next));
  };
  return (
    <div class="flex flex-col gap-2">
      <fieldset class="m-0 flex min-w-0 flex-wrap gap-1.5 border-0 p-0">
        <legend class="sr-only">Days</legend>
        <For each={DAYS}>
          {(day) => (
            <button
              type="button"
              aria-pressed={on(day.value)}
              title={day.long}
              onClick={() => flip(day.value)}
              class={cx(CHIP, on(day.value) ? CHIP_ON : CHIP_OFF)}
            >
              {day.short}
            </button>
          )}
        </For>
      </fieldset>
      <div class="flex flex-wrap gap-1.5">
        <Button size={28} variant="quiet" onClick={() => props.form.setDays([])}>
          Every day
        </Button>
        <Button size={28} variant="quiet" onClick={() => props.form.setDays([...WEEKDAYS])}>
          Weekdays
        </Button>
        <Button size={28} variant="quiet" onClick={() => props.form.setDays([...WEEKEND])}>
          Weekends
        </Button>
      </div>
    </div>
  );
}

/** When it runs: a time and days for Cron, a number and unit for Interval, words for the others. */
function WhenPicker(props: { form: ScheduleForm; error: string | null | undefined }) {
  const form = props.form;
  return (
    <div class="flex flex-col gap-2.5 col-span-full">
      <span class="font-medium">When</span>
      <Show when={form.trigger() === "Cron"}>
        <div class="flex flex-wrap items-center gap-3">
          <Input
            type="time"
            aria-label="Time"
            class="w-32"
            value={form.time()}
            onInput={(event) => form.setTime(event.currentTarget.value)}
          />
          <DayPicker form={form} />
        </div>
      </Show>
      <Show when={form.trigger() === "Interval"}>
        <div class="flex flex-wrap items-center gap-2">
          <span>Every</span>
          <Input
            type="number"
            min={1}
            aria-label="Interval"
            class="w-24"
            value={form.every()}
            onInput={(event) => form.setEvery(event.currentTarget.valueAsNumber)}
          />
          <Select
            aria-label="Interval unit"
            options={["minutes", "hours"]}
            value={form.unit()}
            onChange={(event) =>
              form.setUnit(event.currentTarget.value === "hours" ? "hours" : "minutes")
            }
          />
        </div>
      </Show>
      <Show when={form.trigger() === "One-time" || form.trigger() === "Event"}>
        <Input
          aria-label="When, in words"
          value={form.whenText()}
          invalid={!!props.error}
          onInput={(event) => form.setWhenText(event.currentTarget.value)}
        />
        <Show when={!props.error}>
          <span class="text-small leading-4.5 text-secondary">{WORDS[form.trigger()]?.hint}</span>
        </Show>
      </Show>
      <Show when={form.trigger() === "Cron" || form.trigger() === "Interval"}>
        <span class="text-small text-secondary">
          Runs: <strong class="text-primary">{form.when()}</strong>
        </span>
      </Show>
      <Show when={props.error}>
        <span class="text-small leading-4.5 text-status-danger-text">{props.error}</span>
      </Show>
    </div>
  );
}

const ROW = "flex items-start gap-2 min-h-7 phone:min-h-(--control-h) cursor-pointer";

/** What a brief is made of, as a checklist, in the order it is written. */
function SectionsPicker(props: { form: ScheduleForm; catalog: ScheduleCatalog }) {
  // The order is fixed when the editor opens, so a part does not jump as it is ticked.
  const order = untrack(() => orderedSections(props.form.sections(), props.catalog.sections));
  return (
    <fieldset class="m-0 flex min-w-0 flex-col gap-2 border-0 p-0 col-span-full">
      <legend class="mb-1 p-0 font-medium">What to include</legend>
      <For each={order}>
        {(section) => (
          <label class={ROW}>
            <Checkbox
              align="start"
              checked={props.form.sections().includes(section.id)}
              onChange={() => props.form.setSections(toggled(props.form.sections(), section.id))}
            />
            <span class="flex min-w-0 flex-col">
              <span>{section.label}</span>
              <span class="text-small leading-4.5 text-secondary">{section.hint}</span>
            </span>
          </label>
        )}
      </For>
    </fieldset>
  );
}

/** Where a brief goes besides the run history, with whether each chat is set up. */
function DeliverPicker(props: { form: ScheduleForm; catalog: ScheduleCatalog }) {
  return (
    <fieldset class="m-0 flex min-w-0 flex-col gap-2 border-0 p-0 col-span-full">
      <legend class="mb-1 p-0 font-medium">Send to</legend>
      <span class="text-small leading-4.5 text-secondary">
        The app's run history always keeps it. A chat that is not set up is skipped.
      </span>
      <For each={props.catalog.channels}>
        {(channel) => (
          <label class={ROW}>
            <Checkbox
              align="start"
              checked={props.form.deliver().includes(channel.id)}
              onChange={() => props.form.setDeliver(toggled(props.form.deliver(), channel.id))}
            />
            <span>{channel.label}</span>
            <span class="text-small leading-4.5 text-secondary">
              {connected(channel.id) ? "Connected" : "Not connected yet"}
            </span>
          </label>
        )}
      </For>
      <label class={ROW}>
        <Checkbox
          align="start"
          checked={props.form.quiet()}
          onChange={(event) => props.form.setQuiet(event.currentTarget.checked)}
        />
        <span>Say nothing when there is nothing to report</span>
      </label>
    </fieldset>
  );
}

function save(
  form: ScheduleForm,
  schedule: Schedule,
  catalog: ScheduleCatalog | null,
  edit: EditState,
): void {
  const problem = form.problem();
  if (problem) {
    edit.fail(problem);
    return;
  }
  void M.saveSchedule(schedule.id, form.request(catalog)).then((ok) => {
    if (!ok) return;
    edit.close();
    M.toast("Schedule saved");
  });
}

/**
 * The inline form under a schedule row. The time and days are pickers, so the words and the cron agree;
 * a brief lists what it includes and where it goes, from the daemon's catalog.
 */
export function ScheduleEditor(props: {
  /** The id of the form, so a button outside it (the sheet's header) can submit it. */
  formId?: string;
  schedule: Schedule;
  edit: EditState;
  catalog: ScheduleCatalog | null;
}) {
  const form = untrack(() => createScheduleForm(props.schedule));
  const error = () => props.edit.errorFor(props.schedule.id);
  return (
    <form
      id={props.formId}
      class="flex flex-col gap-4"
      onSubmit={(event) => {
        event.preventDefault();
        save(form, props.schedule, props.catalog, props.edit);
      }}
    >
      <div class={GRID_MIN_200}>
        <Field label="Name">
          <Input value={form.name()} onInput={(event) => form.setName(event.currentTarget.value)} />
        </Field>
        <Field label="Trigger">
          <Select
            options={TRIGGERS}
            value={form.trigger()}
            onChange={(event) => form.setTrigger(event.currentTarget.value)}
          />
        </Field>
        <Field label="If the machine was asleep">
          <Select
            options={MISSED_RUNS}
            value={form.missed()}
            onChange={(event) => form.setMissed(event.currentTarget.value)}
          />
        </Field>
        <WhenPicker form={form} error={error()} />
        <Show when={form.isBrief && props.catalog}>
          {(catalog) => (
            <>
              <SectionsPicker form={form} catalog={catalog()} />
              <DeliverPicker form={form} catalog={catalog()} />
            </>
          )}
        </Show>
        <Show when={!form.isBrief}>
          <Field label="Action" class="col-span-full">
            <Input
              value={form.action()}
              onInput={(event) => form.setAction(event.currentTarget.value)}
            />
          </Field>
          <span class="col-span-full text-small leading-4.5 text-status-needs-you-text">
            This kind of schedule cannot run yet. When it fires it records a run that did nothing.
          </span>
        </Show>
      </div>
      <div class="flex gap-2">
        <Button variant="primary" type="submit">
          Save schedule
        </Button>
        <Button onClick={props.edit.close}>Cancel</Button>
      </div>
    </form>
  );
}
