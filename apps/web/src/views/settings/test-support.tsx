import { render } from "@solidjs/testing-library";
import { batch } from "solid-js";
import { unwrap } from "solid-js/store";
import { M, type State } from "~/mock";
import { createTestMarshal } from "~/testing/test-store";
import { SettingsView } from "./SettingsView";

/** The store fields the settings page reads or writes. */
const VIEWPORT_HEIGHT_PX = 900;
const SETTINGS_KEYS = [
  "roles",
  "providers",
  "integrations",
  "schedules",
  "limits",
  "sleep",
  "profile",
  "projects",
  "settingsSection",
  "roleSel",
  "settingsPid",
  "schedEdit",
  "dialog",
  "toasts",
  "route",
  "removeProject",
  "palette",
  "tour",
  "theme",
] as const satisfies readonly (keyof State)[];

export const DESKTOP_WIDTH_PX = 1440;
export const PHONE_WIDTH_PX = 390;

/* A second store with the simulation off gives untouched seed data to copy from. */
const pristine = unwrap(
  createTestMarshal({
    hash: "#nosim",
    storage: null,
    viewport: { w: DESKTOP_WIDTH_PX, h: 900 },
    applyTheme: () => {},
  }).S,
);

/** Puts the settings fields of the shared store back to the seed, and sets the viewport width. */
function resetSettingsState(width = DESKTOP_WIDTH_PX): void {
  batch(() => {
    for (const key of SETTINGS_KEYS) Reflect.set(M.S, key, structuredClone(pristine[key]));
    M.setViewport(width, VIEWPORT_HEIGHT_PX);
  });
}

/** Renders the Settings view on one section, over fresh seed data. */
export function showSettings(section: string, patch: Partial<State> = {}, width?: number) {
  resetSettingsState(width);
  M.set({ settingsSection: section, ...patch });
  return render(() => <SettingsView />);
}

/** Runs the open confirm dialog's action, as its button would. */
export function confirmDialog(): void {
  const dialog = M.S.dialog;
  if (!dialog) throw new Error("no dialog is open");
  dialog.run();
}

export const lastToast = (): string | undefined => M.S.toasts.at(-1)?.msg;
