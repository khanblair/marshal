import type { ScheduleCatalog } from "@marshal/protocol";
import { cx, Icon, IconButton, Switch } from "@marshal/ui";
import { Show } from "solid-js";
import { zoneLabel } from "~/data/zone";
import { M, type Schedule } from "~/mock";
import { requestFrom } from "~/sync/schedule-actions";

const connected = (channel: string): boolean =>
  M.S.integrations.find((row) => row.id === channel)?.st === "connected";

function turn(schedule: Schedule, enabled: boolean): void {
  void M.saveSchedule(schedule.id, requestFrom(schedule, { enabled })).then((ok) => {
    if (ok) M.toast(enabled ? "Schedule turned on" : "Schedule turned off");
  });
}

/** Removes a schedule, after asking. Its run history goes with it. */
function remove(schedule: Schedule): void {
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

/** When a schedule runs, with the zone after a clock time: "Every day at 8:00 EAT". */
export function whenLine(schedule: Schedule): string {
  const hasClock = schedule.trigger === "Cron" || schedule.trigger === "One-time";
  return hasClock ? `${schedule.when} ${zoneLabel()}` : schedule.when;
}

/** The chats a brief is sent to, by name, for a line under it. */
export function sendsTo(schedule: Schedule, catalog: ScheduleCatalog | null): string {
  if (!catalog || schedule.kind !== "brief") return "";
  const names = (schedule.deliver ?? []).map(
    (id) => catalog.channels.find((channel) => channel.id === id)?.label ?? id,
  );
  return names.length === 0 ? "Kept in History only" : `Sends to ${names.join(", ")}`;
}

/**
 * Runs the schedule now. A brief that goes to a chat that is set up really sends, so it asks first and
 * names the chats; one with nowhere to go runs at once.
 */
function runNow(schedule: Schedule, catalog: ScheduleCatalog | null, shown: () => void): void {
  const live = (schedule.deliver ?? [])
    .filter(connected)
    .map((id) => catalog?.channels.find((channel) => channel.id === id)?.label ?? id);
  const run = (): void => {
    void M.runSchedule(schedule.id).then((result) => {
      if (!result) {
        M.toast("The schedule could not be run");
        return;
      }
      M.toast(result.status === "success" ? "Run finished" : `Run ${result.status}`);
      shown();
    });
  };
  if (live.length === 0) {
    run();
    return;
  }
  M.confirm({
    title: `Run "${schedule.name}" now`,
    message: `It will be sent to ${live.join(", ")} now, as it would at its time.`,
    action: "Run now",
    destructive: false,
    run,
  });
}

interface ScheduleRowProps {
  schedule: Schedule;
  catalog: ScheduleCatalog | null;
  onEdit: () => void;
  onHistory: () => void;
  onPreview: () => void;
}

/**
 * One schedule: what it is called, when it runs, what it does and where it goes, the switch, and its
 * actions as icons, right-aligned on a line of their own on a phone and beside the switch when there is
 * room. Editing, history, and the preview open in a sheet.
 */
export function ScheduleRow(props: ScheduleRowProps) {
  const s = () => props.schedule;
  const name = () => s().name;
  return (
    <div
      data-schedule={s().id}
      class="@container flex flex-col gap-2 py-3.5 px-4 border-b border-border"
    >
      <div class="grid grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-x-3 gap-y-2 @min-[640px]:grid-cols-[auto_minmax(0,1fr)_auto_auto]">
        <Icon name={s().icon} size={18} class="mt-0.5 flex-none" />
        <div class="flex min-w-0 flex-col">
          <span class="font-semibold">{name()}</span>
          <span class="text-small leading-4.5 text-secondary">{whenLine(s())}</span>
          <span class="text-small leading-4.5 text-secondary">{s().action}</span>
          <Show when={sendsTo(s(), props.catalog)}>
            {(line) => <span class="text-caption leading-4 text-muted">{line()}</span>}
          </Show>
          <Show when={s().kind !== "brief"}>
            <span class="text-caption leading-4 text-status-needs-you-text">
              Cannot run yet: a run records that nothing was done.
            </span>
          </Show>
        </div>
        <label
          class={cx(
            "inline-flex items-center gap-2 cursor-pointer text-small",
            "min-h-7 phone:min-h-(--control-h)",
          )}
        >
          <Switch
            checked={s().enabled}
            label={`${s().enabled ? "Turn off " : "Turn on "}${name()}`}
            onCheckedChange={(enabled) => turn(s(), enabled)}
          />
          {s().enabled ? "On" : "Off"}
        </label>
        <div class="col-span-3 flex justify-end gap-1 @min-[640px]:col-span-1">
          <Show when={s().kind === "brief" && props.catalog}>
            <IconButton
              variant="outline"
              icon="eye"
              label={`Preview ${name()}`}
              title="Preview the message"
              onClick={props.onPreview}
            />
            <IconButton
              variant="outline"
              icon="play"
              label={`Run ${name()} now`}
              title="Run now"
              onClick={() => runNow(s(), props.catalog, props.onHistory)}
            />
          </Show>
          <IconButton
            variant="outline"
            icon="history"
            label={`History of ${name()}`}
            title="History"
            onClick={props.onHistory}
          />
          <IconButton
            variant="outline"
            icon="pencil"
            label={`Edit ${name()}`}
            title="Edit"
            onClick={props.onEdit}
          />
          <IconButton
            variant="outline"
            tone="danger"
            icon="trash-2"
            label={`Delete ${name()}`}
            title="Delete"
            onClick={() => remove(s())}
          />
        </div>
      </div>
    </div>
  );
}
