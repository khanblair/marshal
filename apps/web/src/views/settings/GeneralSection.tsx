import { Button, Field, Icon, ItemText, Select, SettingsPanel, SettingsSection } from "@marshal/ui";
import { M } from "~/mock";
import { GRID_MIN_220 } from "./auto-fit-grid";
import { ThemeCards } from "./ThemeCards";

const RESTORE_OPTIONS = ["Auto-restore on startup", "Show a resume button on each card"];
const IDLE_OPTIONS = [
  { value: "5", label: "5 minutes" },
  { value: "15", label: "15 minutes" },
  { value: "30", label: "30 minutes" },
  { value: "60", label: "1 hour" },
];
const CHANNEL_OPTIONS = ["In app only", "In app and Telegram", "In app and Discord"];

function SessionSettings() {
  return (
    <SettingsPanel class={`${GRID_MIN_220} p-4`}>
      <Field label="After a restart" hint="Cards that were awake pick up where they left off.">
        <Select
          options={RESTORE_OPTIONS}
          value={M.S.sleep.restore}
          onChange={(e) => {
            M.S.sleep.restore = e.currentTarget.value;
            M.toast("Saved");
          }}
        />
      </Field>
      <Field label="Sleep idle cards after" hint="You get a notice 2 minutes before a card sleeps.">
        <Select
          options={IDLE_OPTIONS}
          value={String(M.S.sleep.idle)}
          onChange={(e) => {
            M.S.sleep.idle = +e.currentTarget.value;
            M.toast("Saved");
          }}
        />
      </Field>
      <Field label="Sleep warnings go to">
        <Select
          options={CHANNEL_OPTIONS}
          value={M.S.sleep.channel}
          onChange={(e) => {
            M.S.sleep.channel = e.currentTarget.value;
            M.toast("Saved");
          }}
        />
      </Field>
    </SettingsPanel>
  );
}

/** General: the theme cards, the custom theme row, and the session options. */
export function GeneralSection() {
  return (
    <SettingsSection title="Theme">
      <ThemeCards />
      <SettingsPanel class="flex flex-wrap items-center gap-3 py-3 px-4">
        <Icon name="palette" size={16} />
        <ItemText
          basis={240}
          tight
          title="Custom theme"
          description="Override neutrals, ink, and status colors. A custom theme must pass the contrast checks before you can save it."
        />
        <Button onClick={() => M.toast("Custom theme editor opened")}>Create custom theme</Button>
      </SettingsPanel>
      <h2 class="mt-2 mb-0 text-view-title leading-7 font-bold">Sessions</h2>
      <SessionSettings />
    </SettingsSection>
  );
}
