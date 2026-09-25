import { Badge, IconLabel, ProgressBar, StatusDot, StatusLabel } from "@marshal/ui";
import { Show } from "solid-js";
import type { CardView } from "~/mock";
import { CardFooter } from "./CardFooter";

export interface CardItemProps {
  /** The card's view model, `M.deco(card)`. Read through props so it stays reactive. */
  c: CardView;
}

function CardTitle(props: CardItemProps) {
  return (
    <div class="flex items-start gap-2">
      <span
        class="flex-1 min-w-0 font-semibold line-clamp-2 text-pretty"
        style={{ color: props.c.titleColor }}
      >
        {props.c.title}
      </span>
      <span class="flex-none text-caption leading-5 text-muted">{props.c.num}</span>
    </div>
  );
}

function StateLine(props: CardItemProps) {
  return (
    <div class="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-small leading-4.5 text-secondary">
      <StatusLabel state={props.c.state}>{props.c.stateLabel}</StatusLabel>
      <Show when={props.c.asleep}>
        <IconLabel icon="moon" gap={3}>
          {props.c.sleepLabel}
        </IconLabel>
      </Show>
      <span>{props.c.role}</span>
      <Show when={props.c.hasModel}>
        <span>{props.c.model}</span>
      </Show>
      <Show when={props.c.hasThink}>
        <span>{props.c.think}</span>
      </Show>
    </div>
  );
}

/** What the agent is doing, why it needs you, or that you paused it. */
function Activity(props: CardItemProps) {
  return (
    <>
      <Show when={props.c.showDoing}>
        <div class="flex items-center gap-2 text-small leading-4.5 text-primary">
          <StatusDot state={props.c.state} />
          <span class="flex-1 min-w-0 truncate">{props.c.doing}</span>
        </div>
      </Show>
      <Show when={props.c.paused}>
        <IconLabel icon="pause" gap={6} class="text-small leading-4.5 text-secondary">
          Paused by you
        </IconLabel>
      </Show>
      <Show when={props.c.showReason}>
        <div class="text-small leading-4.5 text-status-needs-you-text font-medium line-clamp-2">
          {props.c.reason}
        </div>
      </Show>
    </>
  );
}

/**
 * One card on the board: state, role and model, what it is doing, merge progress, package, and
 * a footer of meta. Port of design/CardItem.dc.html. The status edge, the selected and focus
 * ring, and the dragging fade come from the view model as inline colors.
 */
export function CardItem(props: CardItemProps) {
  return (
    // biome-ignore lint/a11y/useSemanticElements: the card can hold buttons and starts a pointer drag, so it cannot be a <button>
    <div
      data-card={props.c.id}
      tabIndex={0}
      role="button"
      aria-label={props.c.aria}
      onClick={() => props.c.open()}
      onPointerDown={(e) => props.c.down(e)}
      onKeyDown={(e) => props.c.key2(e)}
      class="relative flex flex-col gap-2 p-3 bg-surface border border-border border-l-3 rounded-md cursor-pointer select-none transition-[border-color,opacity] duration-[140ms,200ms] ease-standard hover:border-t-border-strong hover:border-r-border-strong hover:border-b-border-strong"
      style={{
        "border-left-color": props.c.edge,
        "box-shadow": props.c.ring,
        opacity: props.c.opacity,
      }}
    >
      <Show when={props.c.bypass}>
        <Badge tone="bypass" icon="shield-alert" class="self-start leading-4">
          Bypass
        </Badge>
      </Show>
      <CardTitle c={props.c} />
      <StateLine c={props.c} />
      <Activity c={props.c} />
      <Show when={props.c.merging}>
        <ProgressBar value={props.c.mergeNum} label="Merge progress" tone="ready" size={4} />
      </Show>
      <Show when={props.c.hasPkg}>
        <Badge tone="outline" icon="package" class="self-start font-mono font-normal! leading-4">
          {props.c.pkg}
        </Badge>
      </Show>
      <Show when={props.c.hasFooter}>
        <CardFooter c={props.c} />
      </Show>
    </div>
  );
}
