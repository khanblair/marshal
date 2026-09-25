import { cx, Icon, toneSolidText, toneText } from "@marshal/ui";
import { For } from "solid-js";
import { DEMO_COLUMNS } from "./onboarding-data";
import { StepIntro } from "./StepIntro";

/** Screen 1: what Marshal does, with a small board of placeholder cards. */
export function WelcomeStep() {
  return (
    <>
      <StepIntro class="max-w-[60ch]">
        Marshal runs your AI coding agents on a board, one card per task, and moves each card as the
        work really progresses.
      </StepIntro>
      <div
        role="img"
        aria-label="A board with cards moving from planning to done"
        class="grid grid-cols-4 gap-2 p-3 rounded-lg bg-surface-sunken"
      >
        <For each={DEMO_COLUMNS}>
          {(column) => (
            <div class="flex flex-col gap-1.5 min-w-0">
              <span
                class={cx(
                  "flex items-center gap-1 text-caption font-semibold",
                  toneText[column.tone],
                )}
              >
                <span class={cx("inline-flex", toneSolidText[column.tone])}>
                  <Icon name={column.icon} size={12} />
                </span>
                {column.label}
              </span>
              <For each={column.cards}>
                {(width) => (
                  <span
                    class={cx(
                      "h-8.5 rounded-sm border border-border border-l-3 bg-surface px-2 py-1.5",
                      column.edge,
                    )}
                  >
                    <span class="block h-1.5 rounded-2xs bg-border-strong" style={{ width }} />
                  </span>
                )}
              </For>
            </div>
          )}
        </For>
      </div>
    </>
  );
}
