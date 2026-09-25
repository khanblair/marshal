import { cx, Icon } from "@marshal/ui";
import { For, Match, Switch } from "solid-js";
import { M } from "~/mock";
import { AgentsStep } from "./AgentsStep";
import { ControlStep } from "./ControlStep";
import { OnboardingFooter } from "./OnboardingFooter";
import {
  AGENTS_STEP,
  CONTROL_STEP,
  PROFILE_STEP,
  PROJECT_STEP,
  STEP_COUNT,
  STEP_TITLES,
  WELCOME_STEP,
} from "./onboarding-data";
import { createOnboardingFlow } from "./onboarding-flow";
import { ProfileStep } from "./ProfileStep";
import { ProjectStep } from "./ProjectStep";
import { WelcomeStep } from "./WelcomeStep";

const DOTS = Array.from({ length: STEP_COUNT }, (_, index) => index);

/**
 * First-launch setup: five screens in one centered card (full screen on phones), with a
 * step indicator, Skip, Back, and Continue.
 */
export function Onboarding() {
  const flow = createOnboardingFlow();
  const shared = { draft: flow.draft, setDraft: flow.setDraft };
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="ob-title"
      class={cx(
        "absolute inset-0 overflow-auto bg-canvas flex justify-center",
        M.mobile ? "items-stretch" : "items-center px-4 py-8",
      )}
    >
      <div
        class={cx(
          "w-full max-w-160 flex flex-col gap-5 bg-surface",
          M.mobile ? "px-4 py-5" : "p-7 rounded-xl border border-border",
        )}
      >
        <div class="flex items-center gap-2.5">
          <Icon name="st-done" size={20} />
          <span class="font-bold">Marshal</span>
          <span class="flex-1" />
          <span aria-live="polite" class="text-small text-secondary">
            {flow.stepLabel()}
          </span>
        </div>
        <div aria-hidden="true" class="grid grid-cols-[repeat(5,1fr)] gap-1">
          <For each={DOTS}>
            {(dot) => (
              <span class={cx("h-1 rounded-full", dot <= flow.step() ? "bg-ink" : "bg-border")} />
            )}
          </For>
        </div>
        <h1 id="ob-title" class="m-0 text-view-title leading-7 font-bold">
          {STEP_TITLES[flow.step()]}
        </h1>
        <Switch>
          <Match when={flow.step() === WELCOME_STEP}>
            <WelcomeStep />
          </Match>
          <Match when={flow.step() === PROFILE_STEP}>
            <ProfileStep {...shared} nameError={flow.nameError()} />
          </Match>
          <Match when={flow.step() === AGENTS_STEP}>
            <AgentsStep {...shared} />
          </Match>
          <Match when={flow.step() === PROJECT_STEP}>
            <ProjectStep {...shared} />
          </Match>
          <Match when={flow.step() === CONTROL_STEP}>
            <ControlStep {...shared} />
          </Match>
        </Switch>
        <OnboardingFooter flow={flow} />
      </div>
    </div>
  );
}
