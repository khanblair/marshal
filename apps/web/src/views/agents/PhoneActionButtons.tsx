import { Icon } from "@marshal/ui";
import { Index } from "solid-js";
import type { AgentAction } from "./agent-actions";

export interface PhoneActionButtonsProps {
  actions: readonly AgentAction[];
}

const ICON_PX = 14;

/** The action buttons under a phone row: outlined, 44 px high, still small text. */
export function PhoneActionButtons(props: PhoneActionButtonsProps) {
  return (
    <div class="flex flex-wrap gap-1.5">
      <Index each={props.actions}>
        {(action) => (
          <button
            type="button"
            aria-label={action().aria}
            data-compact="1"
            onClick={(e) => {
              e.stopPropagation();
              action().run();
            }}
            class="min-h-11 px-3 rounded-sm border border-border-strong bg-surface text-small inline-flex items-center gap-1.5"
          >
            <Icon name={action().icon} size={ICON_PX} />
            {action().label}
          </button>
        )}
      </Index>
    </div>
  );
}
