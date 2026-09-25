import { Button, cx } from "@marshal/ui";
import { createMemo, Show } from "solid-js";
import { M } from "~/mock";
import type { TourController } from "./tour-controller";
import { placePopover } from "./tour-geometry";
import { stepAt } from "./tour-steps";

export interface TourPopoverProps {
  tour: TourController;
}

const BASE = "absolute pointer-events-auto flex flex-col gap-1.5 bg-surface-raised shadow-e2";
/** On phones the popover is a sheet along the bottom edge, padded for the home indicator. */
const SHEET =
  "inset-x-0 bottom-0 px-4 pt-4 pb-[calc(16px+env(safe-area-inset-bottom))] rounded-t-xl";
const PANEL = "p-4 rounded-lg border border-border";

/** The step's card: on phones a bottom sheet, elsewhere placed next to the target. */
export function TourPopover(props: TourPopoverProps) {
  const step = () => stepAt(props.tour.step());
  const placement = createMemo(() =>
    placePopover({ rect: props.tour.rect(), vw: M.S.vw, vh: M.S.vh }),
  );
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="tour-title"
      aria-describedby="tour-body"
      class={cx(BASE, M.mobile ? SHEET : PANEL)}
      style={
        M.mobile
          ? undefined
          : {
              left: `${placement().x}px`,
              top: `${placement().y}px`,
              width: `${placement().width}px`,
            }
      }
    >
      <span class="text-small text-secondary">{props.tour.stepLabel()}</span>
      <h2 id="tour-title" class="m-0 text-subtitle leading-5.5 font-semibold">
        {step().title}
      </h2>
      <p id="tour-body" class="m-0 text-secondary">
        {step().body}
      </p>
      <div class="flex items-center gap-2 mt-1">
        <Button variant="quiet" class="px-2! hover:text-secondary!" onClick={props.tour.skip}>
          Skip tour
        </Button>
        <span class="flex-1" />
        <Show when={props.tour.canBack()}>
          <Button class="hover:bg-surface!" onClick={props.tour.back}>
            Back
          </Button>
        </Show>
        <Button
          ref={props.tour.setNextButton}
          variant="primary"
          class="px-3.5! hover:bg-ink!"
          onClick={props.tour.next}
        >
          {props.tour.nextLabel()}
        </Button>
      </div>
    </div>
  );
}
