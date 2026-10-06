import { SettingsPanel, SettingsSection } from "@marshal/ui";
import { createResource, createSignal, For, Show } from "solid-js";
import { M } from "~/mock";
import type { EditState } from "./edit-state";
import { NewScheduleMenu } from "./NewScheduleMenu";
import { ScheduleRow } from "./ScheduleRow";
import { ScheduleSheet, type SheetView } from "./ScheduleSheet";

/**
 * Schedules: the list, each with an on and off switch and its actions as icons. Editing, the history,
 * and the message preview open in a sheet (a bottom sheet on a phone), so the list never stretches.
 */
export function SchedulesSection(props: { edit: EditState }) {
  const [catalog] = createResource(() => M.scheduleCatalog());
  // The editor is the shared edit state's (a new schedule opens it); history and preview are the list's own.
  const [viewing, setViewing] = createSignal<{ id: string; view: SheetView } | null>(null);
  const editing = () => M.S.schedules.find((row) => row.id === props.edit.id());
  const viewed = () => M.S.schedules.find((row) => row.id === viewing()?.id);
  const view = (): SheetView => (editing() ? "edit" : (viewing()?.view ?? "history"));
  const open = () => editing() ?? viewed();
  const close = (): void => {
    props.edit.close();
    setViewing(null);
  };
  const show = (id: string, next: SheetView): void => {
    props.edit.close();
    setViewing({ id, view: next });
  };
  return (
    <SettingsSection
      title="Schedules"
      actions={<NewScheduleMenu catalog={catalog() ?? null} edit={props.edit} />}
    >
      <SettingsPanel list>
        <For each={M.S.schedules}>
          {(schedule) => (
            <ScheduleRow
              schedule={schedule}
              catalog={catalog() ?? null}
              onEdit={() => {
                setViewing(null);
                props.edit.toggle(schedule.id);
              }}
              onHistory={() => show(schedule.id, "history")}
              onPreview={() => show(schedule.id, "preview")}
            />
          )}
        </For>
      </SettingsPanel>
      <Show when={open()}>
        {(schedule) => (
          <ScheduleSheet
            schedule={schedule()}
            view={view()}
            edit={props.edit}
            catalog={catalog() ?? null}
            onClose={close}
          />
        )}
      </Show>
    </SettingsSection>
  );
}
