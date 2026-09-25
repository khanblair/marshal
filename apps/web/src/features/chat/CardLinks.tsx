import { Icon } from "@marshal/ui";
import { Index } from "solid-js";
import type { MsgView } from "~/mock";

export interface CardLinksProps {
  item: MsgView;
}

/** A message followed by one button per card it mentions. */
export function CardLinks(props: CardLinksProps) {
  return (
    <div class="flex flex-col gap-2 max-w-[72ch]">
      <div class="whitespace-pre-wrap text-pretty">{props.item.text}</div>
      <div class="flex flex-col gap-1">
        <Index each={props.item.cards ?? []}>
          {(c) => (
            <button
              type="button"
              onClick={() => c().open()}
              class="self-start inline-flex items-center gap-1.5 min-h-7 py-0.5 px-2 rounded-sm border border-border bg-surface text-left hover:bg-surface-hover"
            >
              <span class="inline-flex" style={{ color: c().iconColor }}>
                <Icon name={c().icon} size={14} />
              </span>
              <span class="font-semibold">{c().num}</span>
              <span>{c().title}</span>
              <span class="text-small" style={{ color: c().stColor }}>
                {c().stateLabel}
              </span>
            </button>
          )}
        </Index>
      </div>
    </div>
  );
}
