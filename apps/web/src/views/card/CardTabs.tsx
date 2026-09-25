import { Index, Show } from "solid-js";
import { type Card, M } from "~/mock";
import { cardTabs, neighborTab } from "./card-tabs";

export interface CardTabsProps {
  card: Card;
}

const TAB =
  "flex-none inline-flex items-center gap-1.5 px-2.5 border-0 border-b-2 bg-transparent hover:text-primary";

/** Moves to the next tab with the arrow keys and gives its button focus. */
function onTabKeys(e: KeyboardEvent) {
  if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
  e.preventDefault();
  e.stopPropagation();
  const next = neighborTab(M.S.tab, e.key);
  M.setTab(next);
  setTimeout(() => document.getElementById(`tab-${next}`)?.focus(), 0);
}

/** The tab strip under the card header. */
export function CardTabs(props: CardTabsProps) {
  return (
    <div
      role="tablist"
      aria-label="Card sections"
      onKeyDown={onTabKeys}
      class="flex gap-0.5 -mx-1 overflow-x-auto"
    >
      <Index each={cardTabs(props.card)}>
        {(tab) => {
          const selected = () => M.S.tab === tab().key;
          return (
            <button
              type="button"
              role="tab"
              id={`tab-${tab().key}`}
              aria-selected={selected()}
              tabindex={selected() ? "0" : "-1"}
              onClick={() => M.setTab(tab().key)}
              class={`${TAB} ${M.mobile ? "h-11" : "h-9"} ${
                selected()
                  ? "border-b-ink font-semibold text-primary"
                  : "border-b-transparent font-medium text-secondary"
              }`}
            >
              {tab().label}
              <Show when={tab().count !== undefined}>
                <span
                  class={`min-w-4.5 h-4.5 px-1.25 rounded-xs text-caption leading-4.5 font-semibold ${
                    tab().failed
                      ? "bg-status-danger-subtle text-status-danger-text"
                      : "bg-surface-selected text-secondary"
                  }`}
                >
                  {tab().count}
                </span>
              </Show>
            </button>
          );
        }}
      </Index>
    </div>
  );
}
