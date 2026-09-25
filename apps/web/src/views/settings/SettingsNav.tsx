import { cx, Icon } from "@marshal/ui";
import { For } from "solid-js";
import { M } from "~/mock";
import { SECTIONS } from "./sections";

/**
 * The section list: a rail on the left, or a scrolling row on top on phones. The buttons keep a
 * minimum height (44 px on phones), left-aligned text, and no wrapping, so they are plain
 * buttons rather than the library's fixed-height ones.
 */
export function SettingsNav() {
  return (
    <nav
      aria-label="Settings sections"
      class={cx(
        "flex-none flex gap-0.5 overflow-auto bg-canvas",
        M.mobile
          ? "flex-row py-2 px-3 border-b border-border"
          : "flex-col w-sidebar py-4 px-2 border-r border-border",
      )}
    >
      <For each={SECTIONS}>
        {(section) => {
          const current = () => M.S.settingsSection === section.key;
          return (
            <button
              type="button"
              aria-current={current() ? "page" : undefined}
              onClick={() => M.set({ settingsSection: section.key })}
              class={cx(
                "flex-none flex items-center gap-2 py-0 px-2.5 border-none rounded-sm text-left whitespace-nowrap hover:bg-surface-hover",
                M.mobile ? "min-h-11" : "min-h-8",
                current() ? "bg-surface-selected font-semibold" : "bg-transparent font-medium",
              )}
            >
              <Icon name={section.icon} size={16} />
              {section.label}
            </button>
          );
        }}
      </For>
    </nav>
  );
}
