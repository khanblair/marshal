import { Icon } from "@marshal/ui";
import { Index } from "solid-js";
import type { AgentAction } from "./agent-actions";

export interface TableActionButtonsProps {
  actions: readonly AgentAction[];
}

const ICON_PX = 14;

/**
 * The Actions cell: quiet text buttons that stop the click so the row does not open. The
 * buttons are matched by position, so the focused one stays put when its label changes
 * (Pin becomes Unpin).
 */
export function TableActionButtons(props: TableActionButtonsProps) {
  return (
    <div class="flex gap-0.5">
      <Index each={props.actions}>
        {(action) => (
          <button
            type="button"
            title={action().label}
            aria-label={action().aria}
            onClick={(e) => {
              e.stopPropagation();
              action().run();
            }}
            class="h-7 px-2 border-none rounded-sm bg-transparent text-secondary text-small inline-flex items-center gap-1 hover:bg-surface-selected hover:text-primary"
          >
            <Icon name={action().icon} size={ICON_PX} />
            {action().label}
          </button>
        )}
      </Index>
    </div>
  );
}
