import type { SetStoreFunction } from "solid-js/store";
import type { OnboardingDraft } from "./onboarding-draft";

/** What each screen needs: the draft to read and the setter to write. */
export interface StepProps {
  draft: OnboardingDraft;
  setDraft: SetStoreFunction<OnboardingDraft>;
}
