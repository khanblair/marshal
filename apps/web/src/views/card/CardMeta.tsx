import { CiStatus, IconLabel, ProgressBar } from "@marshal/ui";
import { Show } from "solid-js";
import type { Card, CardView } from "~/mock";

export interface CardMetaProps {
  card: Card;
  c: CardView;
}

/** Context use turns amber from this share of the window. */
const NEAR_FULL_RATIO = 0.8;
const PERCENT = 100;

/** The context window meter. It turns amber when the window is nearly full. */
function ContextMeter(props: { ctx: number }) {
  const near = () => props.ctx >= NEAR_FULL_RATIO;
  const percent = () => Math.round(props.ctx * PERCENT);
  const tip = () =>
    near()
      ? "Context is near full. Auto-compaction starts at 90%. Add card notes to keep key facts."
      : "Context window used. Auto-compaction starts at 90%.";
  return (
    <span title={tip()} class="inline-flex items-center gap-1.5">
      <span>Context</span>
      <ProgressBar
        meter
        value={percent()}
        label="Context window used"
        tone={near() ? "needs-you" : "neutral"}
        class="w-14"
      />
      <span
        class={near() ? "text-status-needs-you-text font-semibold" : "text-secondary font-normal"}
      >
        {percent()}%
      </span>
    </span>
  );
}

/** Branch, package, pull request, CI, cost, and context, in one wrapping line. */
export function CardMeta(props: CardMetaProps) {
  return (
    <div class="flex flex-wrap items-center gap-x-3.5 gap-y-1.5 text-small leading-4.5 text-secondary">
      <Show when={props.c.hasBranch}>
        <IconLabel icon="git-branch" class="min-w-0">
          <span class="font-mono text-caption">{props.c.branch}</span>
        </IconLabel>
      </Show>
      <Show when={props.c.hasPkg}>
        <IconLabel icon="package">
          <span class="font-mono text-caption">{props.c.pkg}</span>
        </IconLabel>
      </Show>
      <Show when={props.card.pr}>
        <IconLabel icon="git-pull-request">Pull request #{props.card.pr}</IconLabel>
      </Show>
      <Show when={props.c.hasCi}>
        <CiStatus status={props.card.ci ?? ""}>
          CI <span>{props.c.ciLabel.toLowerCase()}</span>
        </CiStatus>
      </Show>
      <IconLabel icon="circle-dollar-sign">{props.c.cost}</IconLabel>
      <ContextMeter ctx={props.card.ctx} />
    </div>
  );
}
