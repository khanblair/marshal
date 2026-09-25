import { Icon } from "@marshal/ui";
import { Show } from "solid-js";
import type { MsgView } from "~/mock";

export interface ToolCallRowProps {
  item: MsgView;
}

/** A tool call the agent made: a one-line summary that opens to the tool's output. */
export function ToolCallRow(props: ToolCallRowProps) {
  return (
    <div class="max-w-[72ch] border border-border rounded-sm bg-surface overflow-hidden">
      <button
        type="button"
        onClick={() => props.item.toggle?.()}
        aria-expanded={props.item.open}
        class="w-full flex items-center gap-2 min-h-8 py-1 px-2.5 border-none bg-transparent text-left hover:bg-surface-hover"
      >
        <span class="inline-flex text-muted">
          <Icon name={props.item.chev ?? "chevron-right"} size={14} />
        </span>
        <span class="inline-flex text-secondary">
          <Icon name={props.item.icon ?? ""} size={14} />
        </span>
        <span class="flex-1 min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">
          <span class="font-medium">{props.item.verb}</span>{" "}
          <span
            class={
              props.item.mono ? "font-mono text-caption text-secondary" : "font-sans text-secondary"
            }
          >
            {props.item.target}
          </span>
        </span>
        <span
          class="flex-none inline-flex items-center gap-1 text-small leading-4.5"
          style={{ color: props.item.resColor }}
        >
          <Icon name={props.item.stIcon ?? "check"} size={14} />
          {props.item.result}
        </span>
      </button>
      <Show when={props.item.open}>
        <div class="border-t border-border bg-surface-sunken py-2 px-3">
          <Show when={props.item.hasDetail}>
            <pre class="m-0 whitespace-pre-wrap font-mono text-small leading-5">
              {props.item.detail}
            </pre>
          </Show>
          <Show when={props.item.noDetail}>
            <span class="text-small text-secondary">
              {props.item.verb} {props.item.target}. Result: {props.item.result}.
            </span>
          </Show>
        </div>
      </Show>
    </div>
  );
}
