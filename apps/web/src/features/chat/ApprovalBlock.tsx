import { Button, Icon } from "@marshal/ui";
import { Show } from "solid-js";
import type { MsgView } from "~/mock";

export interface ApprovalBlockProps {
  item: MsgView;
}

/**
 * A command the agent wants to run. While it waits, Enter on the group approves and
 * Escape denies; Escape stops there so the app's own Escape handling does not also run.
 */
export function ApprovalBlock(props: ApprovalBlockProps) {
  return (
    // biome-ignore lint/a11y/useSemanticElements: the design uses a focusable div group so Enter and Escape decide the approval
    <div
      tabindex="0"
      onKeyDown={(e) => props.item.keys?.(e)}
      role="group"
      aria-label="Approval request"
      class="max-w-[72ch] border border-border-strong rounded-md bg-surface overflow-hidden"
    >
      <div class="flex items-center gap-2 py-2 px-3 bg-status-needs-you-subtle text-status-needs-you-text font-semibold">
        <Icon name="st-needs" size={16} />
        <span class="flex-1">Approval needed</span>
        <Show when={props.item.hasCard}>
          <button
            type="button"
            onClick={() => props.item.openCard?.()}
            class="border-none bg-transparent p-0 text-inherit text-small underline"
          >
            {props.item.cardLabel}
          </button>
        </Show>
      </div>
      <div class="py-2.5 px-3 flex flex-col gap-2">
        <pre class="m-0 py-2 px-2.5 rounded-sm bg-surface-sunken font-mono text-small leading-5 whitespace-pre-wrap break-all">
          {props.item.cmd}
        </pre>
        <span class="text-small leading-4.5 text-secondary">{props.item.why}</span>
      </div>
      <Show when={props.item.waiting}>
        <div class="flex items-center flex-wrap gap-2 px-3 pb-3">
          <Button
            variant="primary"
            kbd="Enter"
            class="gap-2!"
            onClick={() => props.item.approve?.()}
          >
            Approve
          </Button>
          <Button kbd="Esc" class="gap-2!" onClick={() => props.item.deny?.()}>
            Deny
          </Button>
        </div>
      </Show>
      <Show when={props.item.done}>
        <div
          class="flex items-center gap-1.5 px-3 pb-3 text-small font-semibold"
          style={{ color: props.item.resultColor }}
        >
          <Icon name={props.item.resultIcon ?? ""} size={14} />
          {props.item.resultLabel}
        </div>
      </Show>
    </div>
  );
}
