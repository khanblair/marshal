import { Show } from "solid-js";
import { TourCutout } from "./TourCutout";
import { TourPopover } from "./TourPopover";
import { createTourController } from "./tour-controller";

/**
 * The guided tour of Home: a full-screen dim with a hole around the current target, and a
 * card that says what it is (a bottom sheet on phones). The layer takes every click, so the
 * app underneath cannot be used until the tour ends. Targets are the `data-tour` elements.
 */
export function Tour() {
  const tour = createTourController();
  return (
    <div class="absolute inset-0 pointer-events-none">
      <div
        class="absolute inset-0 pointer-events-auto bg-transparent"
        on:click={(event) => event.stopPropagation()}
      />
      <Show
        when={tour.rect()}
        fallback={<div aria-hidden="true" class="absolute inset-0 bg-tour-dim" />}
      >
        {(rect) => <TourCutout rect={rect()} />}
      </Show>
      <TourPopover tour={tour} />
    </div>
  );
}
