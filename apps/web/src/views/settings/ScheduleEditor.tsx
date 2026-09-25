import { Button, cx, Field, Input, Select } from "@marshal/ui";
import { batch, Show, untrack } from "solid-js";
import { M, type Schedule } from "~/mock";
import { GRID_MIN_200 } from "./auto-fit-grid";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";

const TRIGGERS = ["Cron", "Interval", "One-time", "Event"];
const MISSED_RUNS = ["Run once on wake", "Skip"];
/** A time the scheduler can read starts with one of these words. */
const WHEN_PATTERN = /^(every|on|when)\b/i;
const WHEN_ERROR =
  'Marshal can\'t read this time. Start with "Every", "On", or "When", for example "Every weekday at 9:00".';
const WHEN_HINT = 'For example "Every weekday at 9:00" or "Every 30 minutes".';

function saveSchedule(schedule: Schedule, form: HTMLFormElement, edit: EditState): void {
  const when = fieldValue(form, "when").trim();
  if (!WHEN_PATTERN.test(when)) {
    edit.fail(WHEN_ERROR);
    return;
  }
  batch(() => {
    Object.assign(schedule, {
      name: fieldValue(form, "name").trim() || schedule.name,
      trigger: fieldValue(form, "trigger"),
      when,
      missed: fieldValue(form, "missed"),
      action: fieldValue(form, "action").trim() || schedule.action,
    });
    edit.close();
    M.toast("Schedule saved");
  });
}

/**
 * The inline form under a schedule row. The fields are not controlled: they start from the
 * schedule as it is when the form opens and are read when it is saved.
 */
export function ScheduleEditor(props: { schedule: Schedule; edit: EditState }) {
  const initial = untrack(() => ({ ...props.schedule }));
  const error = () => props.edit.errorFor(props.schedule.id);
  return (
    <form
      class={cx(GRID_MIN_200, "px-4 pb-4")}
      onSubmit={(event) => {
        event.preventDefault();
        saveSchedule(props.schedule, event.currentTarget, props.edit);
      }}
    >
      <Field label="Name">
        <Input name="name" value={initial.name} />
      </Field>
      <Field label="Trigger">
        <Select name="trigger" options={TRIGGERS} value={initial.trigger} />
      </Field>
      <Field label="When" hint={error() ? undefined : WHEN_HINT}>
        <Input name="when" value={initial.when} invalid={!!error()} />
        <Show when={error()}>
          <span class="text-small leading-4.5 text-status-danger-text">{error()}</span>
        </Show>
      </Field>
      <Field label="If the machine was asleep">
        <Select name="missed" options={MISSED_RUNS} value={initial.missed} />
      </Field>
      <Field label="Action" class="col-span-full">
        <Input name="action" value={initial.action} />
      </Field>
      <div class="flex gap-2 col-span-full">
        <Button variant="primary" type="submit">
          Save schedule
        </Button>
        <Button onClick={props.edit.close}>Cancel</Button>
      </div>
    </form>
  );
}
