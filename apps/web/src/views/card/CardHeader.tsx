import { Badge, Icon, IconButton, IconLabel } from "@marshal/ui";
import { Show } from "solid-js";
import { type Card, type CardView, M } from "~/mock";
import { CardActions } from "./CardActions";
import { CardMeta } from "./CardMeta";
import { CardTabs } from "./CardTabs";
import { MembersRow } from "./MembersRow";
import type { Panel } from "./panel-state";
import { SessionSettings } from "./SessionSettings";
import { TitleField } from "./TitleField";

export interface CardHeaderProps {
  card: Card;
  c: CardView;
  panel: Panel;
}

function sessionIcon(card: Card): string {
  if (card.asleep) return "moon";
  return M.isAwake(card) ? "sun" : "circle-dashed";
}

/** Card number, state pill, awake label, pinned, and the expand and close buttons. */
function TopRow(props: CardHeaderProps) {
  const expandLabel = () =>
    M.S.detailExpanded ? "Show board beside card" : "Expand card to full view";
  return (
    <div class="flex items-center gap-2 flex-wrap">
      <Show when={M.mobile}>
        <IconButton
          label="Back"
          icon="chevron-left"
          iconSize={20}
          size={44}
          tone="default"
          class="-ml-3"
          onClick={() => M.closeCard()}
        />
      </Show>
      <span class="text-small text-muted">{props.c.num}</span>
      <Badge size={22} style={{ background: props.c.subtle, color: props.c.stColor }}>
        <span class="inline-flex" style={{ color: props.c.iconColor }}>
          <Icon name={props.c.icon} size={12} />
        </span>
        {props.c.stateLabel}
      </Badge>
      <IconLabel icon={sessionIcon(props.card)} size={12} class="text-caption text-secondary">
        {props.c.awakeLabel}
      </IconLabel>
      <Show when={props.c.pinned}>
        <IconLabel icon="pin" size={12} gap={3} class="text-caption text-secondary">
          Pinned
        </IconLabel>
      </Show>
      <span class="flex-1" />
      <Show when={!M.mobile}>
        <IconButton
          label={expandLabel()}
          title={expandLabel()}
          icon={M.S.detailExpanded ? "minimize-2" : "maximize-2"}
          onClick={() => M.set({ detailExpanded: !M.S.detailExpanded })}
        />
        <IconButton label="Close card" title="Close (Esc)" icon="x" onClick={() => M.closeCard()} />
      </Show>
    </div>
  );
}

/** The card's title, meta line, members, actions, settings, and tabs. */
export function CardHeader(props: CardHeaderProps) {
  return (
    <div class="flex-none pt-3 px-4 pb-0 flex flex-col gap-2.5 border-b border-border">
      <TopRow card={props.card} c={props.c} panel={props.panel} />
      <Show
        when={props.panel.state.editingTitle}
        fallback={
          <button
            type="button"
            onClick={() => props.panel.set({ editingTitle: true })}
            title="Edit title"
            class="self-start max-w-full py-0.5 px-1.5 -mx-1.5 border border-transparent rounded-sm bg-transparent text-left text-title leading-6 font-semibold text-pretty hover:border-border"
          >
            {props.c.title}
          </button>
        }
      >
        <TitleField cardId={props.card.id} title={props.c.title} panel={props.panel} />
      </Show>
      <CardMeta card={props.card} c={props.c} />
      <MembersRow card={props.card} panel={props.panel} />
      <CardActions card={props.card} panel={props.panel} />
      <SessionSettings card={props.card} />
      <span class="text-caption leading-4 text-secondary -mt-1">
        Changes take effect on the next turn.
      </span>
      <CardTabs card={props.card} />
    </div>
  );
}
