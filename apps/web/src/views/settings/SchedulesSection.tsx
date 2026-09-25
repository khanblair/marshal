import { Button, Icon, ItemText, SettingsPanel, SettingsSection, Switch } from "@marshal/ui";
import { batch, For, Show } from "solid-js";
import { M, type Schedule } from "~/mock";
import type { EditState } from "./edit-state";
import { ScheduleEditor } from "./ScheduleEditor";

const MONDAY = 1;
const FRIDAY = 5;
const WEEKDAYS = Array.from({ length: FRIDAY - MONDAY + 1 }, (_, i) => MONDAY + i);

function toggleSchedule(schedule: Schedule, enabled: boolean): void {
  batch(() => {
    schedule.enabled = enabled;
    M.toast(enabled ? "Schedule turned on" : "Schedule turned off");
  });
}

/** A new schedule starts off, with its form open, in the project you were last in. */
function addSchedule(edit: EditState): void {
  const id = `s${Date.now()}`;
  batch(() => {
    M.S.schedules.push({
      id,
      name: "New schedule",
      kind: "job",
      icon: "clock",
      trigger: "Cron",
      when: "Every weekday at 9:00",
      time: "09:00",
      days: [...WEEKDAYS],
      action: "Send a message to the Orchestrator",
      project: M.proj(M.S.route.pid)?.name ?? "",
      enabled: false,
      missed: "Skip",
    });
    edit.moveTo(id);
  });
}

function ScheduleItem(props: { schedule: Schedule; edit: EditState }) {
  const editing = () => props.edit.id() === props.schedule.id;
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
        <Button size={28} onClick={() => props.edit.toggle(props.schedule.id)}>
          {editing() ? "Close" : "Edit"}
        </Button>
      </div>
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
