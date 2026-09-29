import type { ScheduleRun } from "@marshal/protocol";
import { Button, Icon, ItemText, SettingsPanel, SettingsSection, Switch } from "@marshal/ui";
import { createSignal, For, Show } from "solid-js";
import { M, type Schedule } from "~/mock";
import { requestFrom } from "~/sync/schedule-actions";
import type { EditState } from "./edit-state";
import { ScheduleEditor } from "./ScheduleEditor";

const MONDAY = 1;
const FRIDAY = 5;
const WEEKDAYS = Array.from({ length: FRIDAY - MONDAY + 1 }, (_, i) => MONDAY + i);

/** How many of a schedule's latest runs are listed. */
const RECENT_RUNS = 5;

function toggleSchedule(schedule: Schedule, enabled: boolean): void {
  void M.saveSchedule(schedule.id, requestFrom(schedule, { enabled })).then((ok) => {
    if (ok) M.toast(enabled ? "Schedule turned on" : "Schedule turned off");
  });
}

/** Removes a schedule, after asking. Its run history goes with it. */
function removeSchedule(schedule: Schedule): void {
  M.confirm({
    title: `Delete "${schedule.name}"`,
    message: "This removes the schedule and its run history. It cannot be undone.",
    action: "Delete",
    destructive: true,
    run: () => {
      void M.deleteSchedule(schedule.id).then((ok) => {
        if (ok) M.toast("Schedule deleted");
      });
    },
  });
}

/** A new schedule starts off, with its form open, in the project you were last in. */
function addSchedule(edit: EditState): void {
  void M.createSchedule({
    project: M.S.route.pid ?? "",
    name: "New schedule",
    kind: "job",
    icon: "clock",
    trigger: "Cron",
    when: "Every weekday at 9:00",
    time: "09:00",
    days: [...WEEKDAYS],
    action: "Send a message to the Orchestrator",
    enabled: false,
    missed: "Skip",
  }).then((created) => {
    if (created) edit.moveTo(created.id);
  });
}

/** The last run's own line: when it fired, whether it worked, and what it left behind - a
 * brief's own composed text, for a brief. */
function LastRun(props: { run: ScheduleRun }) {
  return (
    <div class="px-4 pb-3 text-small text-secondary whitespace-pre-wrap">
      <span class={props.run.status === "failed" ? "text-status-danger-text" : ""}>
        {new Date(props.run.runAt).toLocaleString()} - {props.run.status}
      </span>
      <Show when={props.run.details}>
        <div class="mt-1">{props.run.details}</div>
      </Show>
    </div>
  );
}

function ScheduleItem(props: { schedule: Schedule; edit: EditState }) {
  const editing = () => props.edit.id() === props.schedule.id;
  const [runs, setRuns] = createSignal<ScheduleRun[] | null>(null);
  const toggleHistory = (): void => {
    if (runs() !== null) {
      setRuns(null);
      return;
    }
    void M.scheduleRuns(props.schedule.id).then(setRuns);
  };
  return (
    <div class="border-b border-border">
      <div class="flex flex-wrap items-center gap-x-3 gap-y-2 py-3 px-4">
        <Icon name={props.schedule.icon} size={16} />
        <ItemText
          basis={220}
          tight
          title={props.schedule.name}
          description={props.schedule.when}
          detail={props.schedule.action}
        />
        <span class="text-small text-secondary">{props.schedule.project}</span>
        <label class="inline-flex items-center gap-2 cursor-pointer text-small">
          <Switch
            checked={props.schedule.enabled}
            label={`${props.schedule.enabled ? "Turn off " : "Turn on "}${props.schedule.name}`}
            onCheckedChange={(enabled) => toggleSchedule(props.schedule, enabled)}
          />
          {props.schedule.enabled ? "On" : "Off"}
        </label>
        <Button size={28} onClick={toggleHistory}>
          {runs() !== null ? "Hide history" : "History"}
        </Button>
        <Button size={28} onClick={() => props.edit.toggle(props.schedule.id)}>
          {editing() ? "Close" : "Edit"}
        </Button>
        <Button size={28} variant="destructive" onClick={() => removeSchedule(props.schedule)}>
          Delete
        </Button>
      </div>
      <Show when={runs()}>
        {(list) => (
          <Show
            when={list().length > 0}
            fallback={<div class="px-4 pb-3 text-small text-secondary">Never run yet.</div>}
          >
            <For each={list().slice(0, RECENT_RUNS)}>{(run) => <LastRun run={run} />}</For>
          </Show>
        )}
      </Show>
      <Show when={editing()}>
        <ScheduleEditor schedule={props.schedule} edit={props.edit} />
      </Show>
    </div>
  );
}

/** Schedules: the list with an on and off switch and an inline editor for each. */
export function SchedulesSection(props: { edit: EditState }) {
  return (
    <SettingsSection
      title="Schedules"
      actions={
        <Button icon="plus" onClick={() => addSchedule(props.edit)}>
          New schedule
        </Button>
      }
    >
      <SettingsPanel list>
        <For each={M.S.schedules}>
          {(schedule) => <ScheduleItem schedule={schedule} edit={props.edit} />}
        </For>
      </SettingsPanel>
    </SettingsSection>
  );
}
