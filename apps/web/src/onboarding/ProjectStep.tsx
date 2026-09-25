import { ChoiceCard, Field, Icon, Input } from "@marshal/ui";
import { For, Show } from "solid-js";
import { SOURCE_CHOICES } from "./onboarding-data";
import { StepIntro } from "./StepIntro";
import type { StepProps } from "./StepProps";

/** Screen 4: pick where the first project comes from. */
export function ProjectStep(props: StepProps) {
  return (
    <>
      <StepIntro>
        Each project is one repository with one board. The sample project is safe to try things on.
      </StepIntro>
      <div
        role="radiogroup"
        aria-label="How to add your first project"
        class="grid grid-cols-[repeat(auto-fit,minmax(170px,1fr))] gap-2.5"
      >
        <For each={SOURCE_CHOICES}>
          {(choice) => (
            <ChoiceCard
              class="gap-1.5 items-start"
              selected={props.draft.source === choice.id}
              onClick={() => props.setDraft("source", choice.id)}
            >
              <Icon name={choice.icon} size={20} />
              <span class="font-semibold">{choice.label}</span>
              <span class="text-small leading-4.5 text-secondary">{choice.desc}</span>
            </ChoiceCard>
          )}
        </For>
      </div>
      <Show when={props.draft.source === "folder"}>
        <Field label="Repository folder">
          <Input
            mono
            value={props.draft.path}
            onInput={(e) => props.setDraft("path", e.currentTarget.value)}
            placeholder="~/code/my-repo"
          />
        </Field>
      </Show>
      <Show when={props.draft.source === "github"}>
        <Field label="Repository URL">
          <Input
            mono
            value={props.draft.url}
            onInput={(e) => props.setDraft("url", e.currentTarget.value)}
            placeholder="https://github.com/owner/repo"
          />
        </Field>
      </Show>
    </>
  );
}
