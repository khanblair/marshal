import { Icon } from "@marshal/ui";
import { createMemo } from "solid-js";
import { type Card, M } from "~/mock";
import { barWidth, barX, px, type Span, TRACK_WIDTH_PX } from "./timeline-geometry";
import { barTip } from "./timeline-model";

export interface TimelineRowProps {
  card: Card;
  /** The bar's days, shifted while it is dragged. */
  span: Span;
  dragging: boolean;
  onBarDown: (e: PointerEvent) => void;
}

const ROW_ICON_PX = 14;
const BAR_ICON_PX = 12;

/** One card: its name in the sticky column and its bar on the day track. */
export function TimelineRow(props: TimelineRowProps) {
  const d = createMemo(() => M.deco(props.card));
  const backlog = () => props.card.state === "backlog";
  const tip = () =>
    barTip(
      props.card,
      d().stateLabel,
      M.S.cards.filter((b) => b.deps.includes(props.card.id)),
    );
  return (
    <div class="flex h-11 border-b border-border">
      <button
        type="button"
        data-card={props.card.id}
        onClick={() => M.openCard(props.card.id)}
        aria-label={d().aria}
        class="sticky left-0 z-raised w-[280px] flex-none flex items-center gap-2 py-0 px-4 border-0 border-r border-border bg-surface text-left hover:bg-surface-hover"
      >
        <Icon name={d().icon} size={ROW_ICON_PX} style={{ color: d().iconColor }} />
        <span class="flex-none text-caption text-muted">{d().num}</span>
        <span
          class="flex-1 min-w-0 overflow-hidden text-ellipsis whitespace-nowrap font-semibold"
          style={{ color: d().titleColor }}
        >
          {props.card.title}
        </span>
      </button>
      <div
        class="relative flex-none [background-image:linear-gradient(to_right,var(--color-border)_1px,transparent_1px)] [background-size:40px_100%]"
        style={{ width: px(TRACK_WIDTH_PX) }}
      >
        {/* biome-ignore lint/a11y/useSemanticElements: the design draws the bar as a div with role button, out of the tab order; the row's name button is the keyboard target */}
        <div
          onPointerDown={(e) => props.onBarDown(e)}
          title={tip()}
          role="button"
          tabIndex={-1}
          class="absolute top-2 h-7 flex items-center gap-1.5 py-0 px-2 rounded-sm border border-l-[3px] text-caption leading-4 font-semibold whitespace-nowrap overflow-hidden cursor-grab select-none touch-none"
          classList={{ "shadow-drag": props.dragging }}
          style={{
            left: px(barX(props.span)),
            width: px(barWidth(props.span)),
            "border-color": backlog() ? "var(--color-border-strong)" : d().edge,
            background: backlog() ? "var(--color-surface-sunken)" : d().subtle,
            color: d().stColor,
          }}
        >
          <Icon name={d().icon} size={BAR_ICON_PX} style={{ color: d().iconColor }} />
          {d().stateLabel}
        </div>
      </div>
    </div>
  );
}
