import type { IconNameInput, ToneKey } from "@marshal/ui";
import type { Theme } from "~/mock";

export const STEP_TITLES = [
  "Welcome to Marshal",
  "Set up your profile",
  "Connect your agents",
  "Add your first project",
  "Stay in control from anywhere",
] as const;

export const STEP_COUNT = STEP_TITLES.length;
export const LAST_STEP = STEP_COUNT - 1;
export const WELCOME_STEP = 0;
export const PROFILE_STEP = 1;
export const AGENTS_STEP = 2;
export const PROJECT_STEP = 3;
export const CONTROL_STEP = 4;

/** Delays in ms: Continue takes focus when the screen opens, and again right after a step change. */
export const FOCUS_ON_OPEN_MS = 60;
export const FOCUS_AFTER_STEP_MS = 30;

interface DemoColumn {
  label: string;
  icon: IconNameInput;
  tone: ToneKey;
  /** The card's left edge in the column's solid color, written out in full for Tailwind. */
  edge: string;
  /** Widths of the placeholder lines on each demo card. */
  cards: readonly string[];
}

/** The four columns of the demo board on the welcome screen. */
export const DEMO_COLUMNS: readonly DemoColumn[] = [
  {
    label: "Planning",
    icon: "st-planning",
    tone: "planning",
    edge: "border-l-status-planning-solid",
    cards: ["70%", "50%"],
  },
  {
    label: "Working",
    icon: "st-working",
    tone: "working",
    edge: "border-l-status-working-solid",
    cards: ["60%", "80%", "45%"],
  },
  {
    label: "Needs you",
    icon: "st-needs",
    tone: "needs-you",
    edge: "border-l-status-needs-you-solid",
    cards: ["55%"],
  },
  {
    label: "Done",
    icon: "st-done",
    tone: "done",
    edge: "border-l-status-done-solid",
    cards: ["65%", "40%"],
  },
];

export const TIME_ZONES = [
  "Europe/London",
  "Europe/Lisbon",
  "Africa/Lagos",
  "America/New_York",
  "America/Los_Angeles",
  "Asia/Singapore",
] as const;

export const THEME_OPTIONS: readonly { value: Theme; label: string; icon: IconNameInput }[] = [
  { value: "light", label: "Light", icon: "sun" },
  { value: "dark", label: "Dark", icon: "moon" },
  { value: "system", label: "System", icon: "monitor" },
];

/** Agents the welcome flow reports as found on this computer. */
export const FOUND_AGENTS = [
  { name: "Claude Code", version: "2.0.14" },
  { name: "Codex", version: "0.42.0" },
  { name: "Gemini CLI", version: "0.8.1" },
] as const;

export type KeyId = "anthropic" | "openai" | "gemini";

/** The provider id doubles as the id in `M.S.providers`. */
export const KEY_FIELDS: readonly { id: KeyId; label: string }[] = [
  { id: "anthropic", label: "Anthropic API key" },
  { id: "openai", label: "OpenAI API key" },
  { id: "gemini", label: "Gemini API key" },
];

export type ProjectSource = "folder" | "github" | "sample";

export const SOURCE_CHOICES: readonly {
  id: ProjectSource;
  label: string;
  icon: IconNameInput;
  desc: string;
}[] = [
  {
    id: "folder",
    label: "Pick a folder",
    icon: "folder-open",
    desc: "A repository already on this computer",
  },
  { id: "github", label: "Clone from GitHub", icon: "github", desc: "Paste a repository URL" },
  {
    id: "sample",
    label: "Use a sample project",
    icon: "flask-conical",
    desc: "Try Marshal on a small sample repository",
  },
];

export const SAMPLE_PROJECT = { name: "marshal-sample", path: "~/.marshal/sample" } as const;

export const PAIRING_CODE = "4K7-Q2M";

export type ChatAppId = "telegram" | "discord";

/** The id doubles as the id in `M.S.integrations`. */
export const CHAT_APPS: readonly {
  id: ChatAppId;
  name: string;
  icon: IconNameInput;
  desc: string;
  button: string;
}[] = [
  {
    id: "telegram",
    name: "Telegram",
    icon: "send",
    desc: "Notices and approval buttons in a Telegram chat",
    button: "Connect Telegram",
  },
  {
    id: "discord",
    name: "Discord",
    icon: "message-circle",
    desc: "Notices and approval buttons in a Discord channel",
    button: "Connect Discord",
  },
];
