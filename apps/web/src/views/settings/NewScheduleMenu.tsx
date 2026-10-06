import type { SaveScheduleRequest, ScheduleCatalog } from "@marshal/protocol";
import { Button, Menu, MenuItem, MenuLabel, MenuSeparator } from "@marshal/ui";
import { createSignal, For, Show } from "solid-js";
import { M } from "~/mock";
import type { EditState } from "./edit-state";
import { custom, fromTemplate } from "./new-schedule";

const SHEET_ROW = 48;
const POPOVER_ROW = 36;
const rowSize = (): typeof SHEET_ROW | typeof POPOVER_ROW => (M.mobile ? SHEET_ROW : POPOVER_ROW);

/**
 * New schedule: the starter templates, so one that was deleted can be added back, and a blank one. A
 * new schedule is switched off with its form open, so the time can be set before it runs.
 */
export function NewScheduleMenu(props: { catalog: ScheduleCatalog | null; edit: EditState }) {
  const [open, setOpen] = createSignal(false);
  let trigger: HTMLButtonElement | undefined;
  const close = () => setOpen(false);
  const add = (request: SaveScheduleRequest, message: string) => {
    close();
    void M.createSchedule(request).then((created) => {
      if (!created) return;
      props.edit.moveTo(created.id);
      M.toast(message);
    });
  };
  return (
    <div class="relative">
      <Button
        ref={(el) => {
          trigger = el;
        }}
        icon="plus"
        aria-haspopup="menu"
        aria-expanded={open()}
        onClick={() => setOpen(!open())}
      >
        New schedule
      </Button>
      <Show when={open()}>
        <Menu
          sheet={M.mobile}
          onClose={close}
          trigger={() => trigger}
          class={M.mobile ? undefined : "absolute top-[calc(100%+4px)] right-0 w-72 z-menu"}
        >
          <Show when={props.catalog}>
            {(catalog) => (
              <>
                <MenuLabel>Start from a template</MenuLabel>
                <For each={catalog().templates}>
                  {(template) => (
                    <MenuItem
                      icon={template.icon}
                      size={rowSize()}
                      title={template.summary}
                      onClick={() =>
                        add(
                          fromTemplate(template, catalog()),
                          `${template.name} added, switched off. Set the time, then turn it on.`,
                        )
                      }
                    >
                      {template.name}
                    </MenuItem>
                  )}
                </For>
                <MenuSeparator />
              </>
            )}
          </Show>
          <MenuItem
            icon="plus"
            size={rowSize()}
            onClick={() => add(custom(props.catalog), "Schedule added, switched off.")}
          >
            Blank schedule
          </MenuItem>
        </Menu>
      </Show>
    </div>
  );
}
