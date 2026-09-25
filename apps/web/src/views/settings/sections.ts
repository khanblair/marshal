import type { IconNameInput } from "@marshal/ui";

export type SectionKey =
  | "profile"
  | "general"
  | "project"
  | "roles"
  | "providers"
  | "limits"
  | "schedules"
  | "integrations"
  | "shortcuts"
  | "help";

export interface SectionSpec {
  key: SectionKey;
  label: string;
  icon: IconNameInput;
}

/** The settings pages, in the order of the section list. */
export const SECTIONS: readonly SectionSpec[] = [
  { key: "profile", label: "Profile", icon: "user-round" },
  { key: "general", label: "General", icon: "sliders-horizontal" },
  { key: "project", label: "Project settings", icon: "folder-cog" },
  { key: "roles", label: "Roles", icon: "users" },
  { key: "providers", label: "Provider keys", icon: "key-round" },
  { key: "limits", label: "Cost and awake limits", icon: "gauge" },
  { key: "schedules", label: "Schedules", icon: "clock" },
  { key: "integrations", label: "Integrations", icon: "plug" },
  { key: "shortcuts", label: "Keyboard shortcuts", icon: "keyboard" },
  { key: "help", label: "Help", icon: "life-buoy" },
];
