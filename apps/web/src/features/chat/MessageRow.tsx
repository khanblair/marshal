import { Icon } from "@marshal/ui";
import { Show } from "solid-js";
import type { MsgView } from "~/mock";
import { ApprovalBlock } from "./ApprovalBlock";
import { CardLinks } from "./CardLinks";
import { CardPreview } from "./CardPreview";
import { DiffSummaryRow } from "./DiffSummaryRow";
import { PlanBlock } from "./PlanBlock";
import { ToolCallRow } from "./ToolCallRow";

export interface MessageRowProps {
  item: MsgView;
}

/** One message. Exactly one kind flag of the view model is set, so exactly one branch renders. */
export function MessageRow(props: MessageRowProps) {
  return (
    <>
      <Show when={props.item.isUser}>
        <div class="self-end max-w-[min(72ch,88%)] py-2 px-3 rounded-lg border border-border bg-surface-sunken whitespace-pre-wrap text-pretty">
          {props.item.text}
        </div>
      </Show>
      <Show when={props.item.isAgent}>
        <div
          aria-busy={props.item.streaming}
          class="max-w-[72ch] whitespace-pre-wrap text-pretty text-primary"
        >
          {props.item.text}
        </div>
      </Show>
      <Show when={props.item.isSystem}>
        <div class="flex items-start gap-2 max-w-[72ch] text-small leading-4.5 text-secondary">
          <span class="inline-flex mt-0.5">
            <Icon name="info" size={14} />
          </span>
          <span>{props.item.text}</span>
        </div>
      </Show>
      <Show when={props.item.isTool}>
        <ToolCallRow item={props.item} />
      </Show>
      <Show when={props.item.isDiff}>
        <DiffSummaryRow item={props.item} />
      </Show>
      <Show when={props.item.isPlan}>
        <PlanBlock item={props.item} />
      </Show>
      <Show when={props.item.isApproval}>
        <ApprovalBlock item={props.item} />
      </Show>
      <Show when={props.item.isCard}>
        <CardPreview item={props.item} />
      </Show>
      <Show when={props.item.isLinks}>
        <CardLinks item={props.item} />
      </Show>
    </>
  );
}
