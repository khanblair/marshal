import { Icon } from "@marshal/ui";
import { Show } from "solid-js";
import type { MsgView } from "~/mock";

export interface CardPreviewProps {
  item: MsgView;
}

/** A card the chat points at: title, state, role, agent, and why it needs you. */
export function CardPreview(props: CardPreviewProps) {
  return (
    <Show when={props.item.c}>
      {(c) => (
        <button
          type="button"
          onClick={() => c().open()}
          aria-label={c().aria}
          class="max-w-105 flex flex-col gap-1.5 py-2.5 px-3 border border-border border-l-[3px] rounded-md bg-surface text-left hover:border-t-border-strong hover:border-r-border-strong hover:border-b-border-strong"
          style={{ "border-left-color": c().edge }}
        >
          <span class="flex gap-2 w-full">
            <span class="flex-1 font-semibold" style={{ color: c().titleColor }}>
              {c().title}
            </span>
            <span class="text-caption text-muted">{c().num}</span>
          </span>
          <span class="flex flex-wrap gap-x-2.5 gap-y-1 text-small leading-4.5 text-secondary">
            <span
              class="inline-flex items-center gap-1 font-semibold"
              style={{ color: c().stColor }}
            >
              <span class="inline-flex" style={{ color: c().iconColor }}>
                <Icon name={c().icon} size={14} />
              </span>
              {c().stateLabel}
            </span>
            <span>{c().role}</span>
            <span>{c().agent}</span>
          </span>
          <Show when={c().showReason}>
            <span class="text-small leading-4.5 text-status-needs-you-text font-medium">
              {c().reason}
            </span>
          </Show>
        </button>
      )}
    </Show>
  );
}
