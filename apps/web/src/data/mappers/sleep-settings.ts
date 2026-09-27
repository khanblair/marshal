import { SleepChannelInApp, SleepRestoreAuto, type SleepSettings } from "@marshal/protocol";

/**
 * The sleep settings as the store and the Settings screen hold them (section S26a, B5.6).
 *
 * `channel` and `restore` are the **labels the form shows**, not the wire values, because the form's
 * two `Select`s are drawn from the labels themselves. `toWireSleepSettings` and `toSleepChoice` are
 * the only two places that know the difference.
 *
 * `warn` and `keepAwake` have no field on the screen: the warning is two minutes and the form's own
 * hint says so, and "Keep awake" is not configurable from Settings. They are still held here, because
 * one save sends the whole record and a value the form never showed must come back unchanged.
 *
 * The shape lives in the data layer, not in `mock/`, so the mapper never depends on the mock;
 * `mock/types.ts` re-exports it for the store's own modules.
 */
export interface SleepChoice {
  /** How long a card's session may be idle before it is warned, in minutes (5, 15, 30, or 60). */
  idle: number;
  /** How long the warning stands before the idle cards sleep, in minutes. */
  warn: number;
  /** How long "Keep awake" holds a card off the idle timer, in minutes. */
  keepAwake: number;
  /** The label of where sleep warnings go. */
  channel: string;
  /** The label of what happens to awake cards after a restart. */
  restore: string;
}

/** The idle times the form offers and the daemon accepts, in minutes (`settings.idleChoices`). */
// biome-ignore lint/style/noMagicNumbers: the choices themselves, not a quantity
export const IDLE_CHOICE_MINUTES: readonly number[] = [5, 15, 30, 60];

/** The form's label for each answer to "After a restart", by the wire value behind it. */
export const RESTORE_LABELS: Readonly<Record<string, string>> = {
  [SleepRestoreAuto]: "Auto-restore on startup",
  manual: "Show a resume button on each card",
};

/** The form's label for each channel a sleep warning can go to, by the wire value behind it. */
export const CHANNEL_LABELS: Readonly<Record<string, string>> = {
  [SleepChannelInApp]: "In app only",
  telegram: "In app and Telegram",
  discord: "In app and Discord",
};

/** The wire value behind a label, or null when the form does not offer that label. */
function wireOf(labels: Readonly<Record<string, string>>, label: string): string | null {
  for (const [value, text] of Object.entries(labels)) if (text === label) return value;
  return null;
}

/**
 * The daemon's settings as the store and the form hold them. A channel or a restore answer this build
 * does not know is kept as it came: the screen shows a `Select` with nothing chosen rather than a
 * wrong label, and a save sends the daemon's own value back instead of quietly changing it.
 */
export function toSleepChoice(settings: SleepSettings): SleepChoice {
  return {
    idle: settings.idleMinutes,
    warn: settings.warningMinutes,
    keepAwake: settings.keepAwakeMinutes,
    channel: CHANNEL_LABELS[settings.channel] ?? settings.channel,
    restore: RESTORE_LABELS[settings.restore] ?? settings.restore,
  };
}

/** The store's settings as the wire wants them, which is one whole record for the PUT. */
export function toWireSleepSettings(choice: SleepChoice): SleepSettings {
  return {
    idleMinutes: choice.idle,
    warningMinutes: choice.warn,
    keepAwakeMinutes: choice.keepAwake,
    restore: wireOf(RESTORE_LABELS, choice.restore) ?? choice.restore,
    channel: wireOf(CHANNEL_LABELS, choice.channel) ?? choice.channel,
  };
}
