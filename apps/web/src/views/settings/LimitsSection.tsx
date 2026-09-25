import { Button, cx, Field, SettingsPanel, SettingsSection } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M } from "~/mock";
import { GRID_MIN_170 } from "./auto-fit-grid";
import {
  GLOBAL_SCOPE,
  type HelpTone,
  LIMIT_SPECS,
  type LimitSpec,
  limitHelp,
  limitUsed,
} from "./limit-rows";
import { NumberInput } from "./NumberInput";
import type { LimitsDraft } from "./use-limits-draft";

const HELP_CLASS: Record<HelpTone, string> = {
  danger: "text-status-danger-text",
  near: "text-status-needs-you-text",
  normal: "text-secondary",
};

function LimitField(props: { spec: LimitSpec; scope: string; draft: LimitsDraft }) {
  const value = () => props.draft.limits()[props.scope]?.[props.spec.key] ?? Number.NaN;
  const help = () => limitHelp(props.spec, limitUsed(props.scope, props.spec.key), value());
  return (
    <Field label={props.spec.label}>
      <span class="flex items-center gap-1.5">
        <Show when={props.spec.money}>
          <span class="text-secondary">$</span>
        </Show>
        <NumberInput
          class="w-full"
          min="0"
          step={props.spec.step}
          value={value()}
          invalid={value() <= 0}
          onInput={(e) => props.draft.set(props.scope, props.spec.key, e.currentTarget.value)}
        />
      </span>
      <span class={cx("text-small leading-4.5", HELP_CLASS[help().tone])}>{help().text}</span>
    </Field>
  );
}

function LimitRow(props: { scope: string; label: string; draft: LimitsDraft }) {
  return (
    <SettingsPanel class="flex flex-col gap-3 py-3.5 px-4">
      <h3 class="m-0 text-subtitle leading-5.5 font-semibold">{props.label}</h3>
      <div class={GRID_MIN_170}>
        <For each={LIMIT_SPECS}>
          {(spec) => <LimitField spec={spec} scope={props.scope} draft={props.draft} />}
        </For>
      </div>
    </SettingsPanel>
  );
}

/** Cost and awake limits: one card of three limits for all projects, then one per project. */
export function LimitsSection(props: { draft: LimitsDraft }) {
  return (
    <SettingsSection
      title="Cost and awake limits"
      description="When a cost limit is reached, running cards pause and move to Needs you. When the awake limit is full, the oldest idle card gets a sleep warning."
    >
      <form
        class="flex flex-col gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          props.draft.save();
        }}
      >
        <LimitRow scope={GLOBAL_SCOPE} label="All projects" draft={props.draft} />
        <For each={M.S.projects}>
          {(project) => <LimitRow scope={project.id} label={project.name} draft={props.draft} />}
        </For>
        <div class="flex gap-2">
          <Button variant="primary" type="submit" disabled={props.draft.unchanged()}>
            Save limits
          </Button>
          <Show when={!props.draft.unchanged()}>
            <Button onClick={props.draft.discard}>Discard changes</Button>
          </Show>
        </div>
      </form>
    </SettingsSection>
  );
}
