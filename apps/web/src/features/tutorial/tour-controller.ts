import { createSignal, onCleanup, onMount } from "solid-js";
import { M } from "~/mock";
import { rectChanged, type TourRect } from "./tour-geometry";
import { stepAt, TOUR_STEPS } from "./tour-steps";
import { measureTarget } from "./tour-target";

/** How often the target is measured again, in ms: pages scroll, resize, and re-render under the tour. */
const MEASURE_INTERVAL_MS = 250;
/** After a step change, the page gets this long to redraw before the target is measured and Next is focused, in ms. */
const SETTLE_MS = 30;
const TOUR_ENDED_MESSAGE = "Tour finished. Replay it from your profile menu.";
const TEXT_FIELD_TAGS = /INPUT|TEXTAREA/;

const isTextField = (el: Element | null): boolean => TEXT_FIELD_TAGS.test(el?.tagName ?? "");

const currentStep = (): number => M.S.tour?.step ?? 0;

/**
 * The current target's box in the app's pixels, measured once when the tour opens (scrolling it
 * into view) and then every 250 ms. It is null while the target is not on screen.
 */
function trackTarget() {
  const [rect, setRect] = createSignal<TourRect | null>(null);
  const measure = (scroll: boolean): void => {
    if (!M.S.tour) return;
    const next = measureTarget(stepAt(currentStep()).targets, M._scale || 1, scroll);
    if (!next) {
      if (rect()) setRect(null);
      return;
    }
    if (rectChanged(rect(), next)) setRect(next);
  };
  onMount(() => {
    const interval = setInterval(() => measure(false), MEASURE_INTERVAL_MS);
    measure(true);
    onCleanup(() => clearInterval(interval));
  });
  return { rect, measure };
}

/**
 * Escape and Enter are read on the window in the capture phase, so nothing underneath sees them
 * first. Enter is left alone while typing in a field. Neither key is stopped from spreading on.
 */
function listenForKeys(keys: { escape: () => void; enter: () => void }): void {
  const onKeyDown = (event: KeyboardEvent): void => {
    if (!M.S.tour) return;
    if (event.key === "Escape") {
      event.preventDefault();
      keys.escape();
    } else if (event.key === "Enter" && !isTextField(document.activeElement)) {
      event.preventDefault();
      keys.enter();
    }
  };
  onMount(() => {
    window.addEventListener("keydown", onKeyDown, true);
    onCleanup(() => window.removeEventListener("keydown", onKeyDown, true));
  });
}

/** The tour's state and actions: the measured target, Next and Back, and ending the tour. */
export function createTourController() {
  const { rect, measure } = trackTarget();
  const timers: ReturnType<typeof setTimeout>[] = [];
  let nextButton: HTMLButtonElement | undefined;
  onCleanup(() => {
    for (const timer of timers) clearTimeout(timer);
  });

  /** How the tour ended: `skipped` from Skip and Escape, `done` from finishing the last step. */
  const end = (status: "done" | "skipped"): void => {
    M.endTour(status);
    M.toast(TOUR_ENDED_MESSAGE);
  };

  const move = (delta: number): void => {
    if (!M.S.tour) return;
    const target = M.S.tour.step + delta;
    if (target < 0) return;
    if (target >= TOUR_STEPS.length) {
      end("done");
      return;
    }
    M.set({ tour: { step: target } });
    timers.push(
      setTimeout(() => {
        measure(true);
        nextButton?.focus();
      }, SETTLE_MS),
    );
  };

  listenForKeys({ escape: () => end("skipped"), enter: () => move(1) });

  return {
    rect,
    step: currentStep,
    stepLabel: (): string => `Step ${currentStep() + 1} of ${TOUR_STEPS.length}`,
    canBack: (): boolean => currentStep() > 0,
    nextLabel: (): string => (currentStep() === TOUR_STEPS.length - 1 ? "Finish tour" : "Next"),
    setNextButton: (el: HTMLButtonElement): void => {
      nextButton = el;
    },
    back: (): void => move(-1),
    next: (): void => move(1),
    skip: (): void => end("skipped"),
  };
}

export type TourController = ReturnType<typeof createTourController>;
