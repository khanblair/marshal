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
import { emailProblem, nameProblem } from "./profile-validation";

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

  const [saving, setSaving] = createSignal(false);
  const step = (): number => M.S.obStep;
  const isLast = (): boolean => step() === LAST_STEP;
  /** Moves to a screen. The step is saved, so a second device opens the onboarding here. */
  const go = (target: number): void => {
    M.setOnboardingStep(target);
    focusContinue(FOCUS_AFTER_STEP_MS);
  };

  /** Finishing needs a named account: a profile that was never filled in sends the person back to it. */
  const finish = (how: "done" | "skipped"): void => {
    if (!M.S.profile.name.trim()) go(PROFILE_STEP);
    else M.finishOnboarding(how);
  };

  return {
    draft,
    setDraft,
    step,
    stepLabel: (): string => `Step ${step() + 1} of ${STEP_COUNT}`,
    isLast,
    canBack: (): boolean => step() > 0,
    continueLabel: (): string => (isLast() ? "Open Marshal" : "Continue"),
    /** The name's problem shows once Continue was pressed, and goes when a good name is typed. */
    nameError: (): string => (step() === PROFILE_STEP && tried() ? nameProblem(draft.name) : ""),
    /** The email's problem shows once Continue was pressed with an email that is not an address. */
    emailError: (): string => (step() === PROFILE_STEP && tried() ? emailProblem(draft.email) : ""),
    /** Setting up the profile is the account, so the profile screen cannot be skipped. */
    canSkip: (): boolean => step() !== PROFILE_STEP,
    /** True while the profile is being saved on the daemon, so Continue is not pressed twice. */
    saving,
    setContinueButton: (el: HTMLButtonElement): void => {
      continueButton = el;
    },
    back: (): void => go(step() - 1),
    skip: (): void => {
      if (step() === PROFILE_STEP) return;
      if (isLast()) finish("skipped");
      else go(step() + 1);
    },
    next: async (): Promise<void> => {
      if (saving()) return;
      if (step() === PROFILE_STEP) {
        setTried(true);
        if (nameProblem(draft.name) || emailProblem(draft.email)) return;
        // The account is saved on the daemon first, and the next screen waits for its answer.
        setSaving(true);
        const saved = await M.saveProfile({
          name: draft.name,
          email: draft.email,
          tz: draft.tz,
        });
        setSaving(false);
        if (!saved) return;
      }
      setTried(false);
      commitStep(step(), draft, M);
      if (isLast()) finish("done");
      else go(step() + 1);
    },
  };
}

export type OnboardingFlow = ReturnType<typeof createOnboardingFlow>;
