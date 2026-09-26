import { createSignal, onCleanup, onMount } from "solid-js";
import { createStore } from "solid-js/store";
import { M } from "~/mock";
import { commitStep } from "./commit-step";
import {
  FOCUS_AFTER_STEP_MS,
  FOCUS_ON_OPEN_MS,
  LAST_STEP,
  PROFILE_STEP,
  STEP_COUNT,
} from "./onboarding-data";
import { initialDraft } from "./onboarding-draft";

/**
 * The state and actions of the five screens: the draft, the current step, and Back,
 * Skip, and Continue. Continue moves focus to itself after each step, as the design does.
 */
export function createOnboardingFlow() {
  const [draft, setDraft] = createStore(initialDraft());
  const [tried, setTried] = createSignal(false);
  const timers: ReturnType<typeof setTimeout>[] = [];
  let continueButton: HTMLButtonElement | undefined;

  const focusContinue = (delayMs: number): void => {
    timers.push(setTimeout(() => continueButton?.focus(), delayMs));
  };
  onMount(() => focusContinue(FOCUS_ON_OPEN_MS));
  onCleanup(() => {
    for (const timer of timers) clearTimeout(timer);
  });

  const step = (): number => M.S.obStep;
  const isLast = (): boolean => step() === LAST_STEP;
  /** Moves to a screen. The step is saved, so a second device opens the onboarding here. */
  const go = (target: number): void => {
    M.setOnboardingStep(target);
    focusContinue(FOCUS_AFTER_STEP_MS);
  };

  return {
    draft,
    setDraft,
    step,
    stepLabel: (): string => `Step ${step() + 1} of ${STEP_COUNT}`,
    isLast,
    canBack: (): boolean => step() > 0,
    continueLabel: (): string => (isLast() ? "Open Marshal" : "Continue"),
    /** The name error shows once Continue was pressed with an empty name, and goes when one is typed. */
    nameError: (): boolean => step() === PROFILE_STEP && tried() && !draft.name.trim(),
    setContinueButton: (el: HTMLButtonElement): void => {
      continueButton = el;
    },
    back: (): void => go(step() - 1),
    skip: (): void => (isLast() ? M.finishOnboarding("skipped") : go(step() + 1)),
    next: (): void => {
      if (step() === PROFILE_STEP && !draft.name.trim()) {
        setTried(true);
        return;
      }
      setTried(false);
      commitStep(step(), draft, M);
      if (isLast()) M.finishOnboarding("done");
      else go(step() + 1);
    },
  };
}

export type OnboardingFlow = ReturnType<typeof createOnboardingFlow>;
