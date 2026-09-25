import { Button } from "@marshal/ui";
import { Show } from "solid-js";
import type { OnboardingFlow } from "./onboarding-flow";

export interface OnboardingFooterProps {
  flow: OnboardingFlow;
}

/**
 * Back, Skip, and Continue. This row stays mounted across screens, so Continue keeps the
 * focus that the flow gives it. The design draws Back without a hover and Skip with a
 * fill only, so those two undo the parts of `Button` that differ.
 */
export function OnboardingFooter(props: OnboardingFooterProps) {
  return (
    <div class="flex items-center gap-2 mt-1 pb-[env(safe-area-inset-bottom)]">
      <Show when={props.flow.canBack()}>
        <Button size={36} class="hover:bg-surface!" onClick={props.flow.back}>
          Back
        </Button>
      </Show>
      <span class="flex-1" />
      <Button
        size={36}
        variant="quiet"
        class="px-3! hover:text-secondary!"
        onClick={props.flow.skip}
      >
        Skip
      </Button>
      <Button
        ref={props.flow.setContinueButton}
        size={36}
        variant="primary"
        class="px-4!"
        onClick={props.flow.next}
      >
        {props.flow.continueLabel()}
      </Button>
    </div>
  );
}
