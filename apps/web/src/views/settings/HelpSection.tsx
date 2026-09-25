import { Button, Icon, type IconNameInput, ItemText, SettingsSection } from "@marshal/ui";
import { For } from "solid-js";
import { M } from "~/mock";

interface HelpItem {
  icon: IconNameInput;
  title: string;
  description: string;
  button: string;
  run: () => void;
}

const HELP_ITEMS: readonly HelpItem[] = [
  {
    icon: "map",
    title: "Tour of Marshal",
    description: "A short walk through Home, projects, views, and notices.",
    button: "Replay tour",
    run: () => M.startTour(),
  },
  {
    icon: "keyboard",
    title: "Keyboard shortcuts",
    description: "Every action has a shortcut or a command in search.",
    button: "Open shortcuts",
    run: () => M.set({ settingsSection: "shortcuts" }),
  },
  {
    icon: "search",
    title: "Search",
    description: "Find actions, projects, cards, and settings in one place.",
    button: "Open search",
    run: () => M.set({ palette: true }),
  },
];

/** Help: replay the tour, and jump to the shortcuts or the search. */
export function HelpSection() {
  return (
    <SettingsSection title="Help" class="gap-3!">
      <For each={HELP_ITEMS}>
        {(item) => (
          <div class="flex flex-wrap items-center gap-3 py-3 border-t border-border">
            <Icon name={item.icon} size={18} />
            <ItemText basis={220} title={item.title} description={item.description} />
            <Button onClick={item.run}>{item.button}</Button>
          </div>
        )}
      </For>
    </SettingsSection>
  );
}
