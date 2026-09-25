import { Kbd, SettingsPanel, SettingsSection } from "@marshal/ui";
import { For } from "solid-js";

interface Shortcut {
  label: string;
  keys: readonly string[];
}

/** ⌘ on Apple devices, Ctrl elsewhere, decided once as the design does. */
const MODIFIER = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl";

const SHORTCUTS: readonly Shortcut[] = [
  { label: "Command palette", keys: [MODIFIER, "K"] },
  { label: "Switch views, in order", keys: [MODIFIER, "1 to 6"] },
  { label: "New card", keys: ["N"] },
  { label: "Focus search or filter", keys: ["/"] },
  { label: "Move between cards and rows", keys: ["Arrow keys"] },
  { label: "Open the selected card", keys: ["Enter"] },
  { label: "Close panel, menu, or dialog", keys: ["Esc"] },
  { label: "Approve the focused approval", keys: ["A"] },
  { label: "Sleep the selected card", keys: ["S"] },
  { label: "Pin or unpin the selected card", keys: ["P"] },
];

/** Keyboard shortcuts: every key the app answers to. */
export function ShortcutsSection() {
  return (
    <SettingsSection
      title="Keyboard shortcuts"
      description="Single-letter shortcuts work when you're not typing in a field."
    >
      <SettingsPanel class="overflow-hidden">
        <For each={SHORTCUTS}>
          {(shortcut) => (
            <div class="flex items-center gap-3 min-h-10 py-1.5 px-4 border-b border-border">
              <span class="flex-1">{shortcut.label}</span>
              <span class="flex gap-1">
                <For each={shortcut.keys}>
                  {(key) => (
                    <Kbd tone="key" class="min-w-6 text-center leading-5! px-1.5!">
                      {key}
                    </Kbd>
                  )}
                </For>
              </span>
            </div>
          )}
        </For>
      </SettingsPanel>
    </SettingsSection>
  );
}
