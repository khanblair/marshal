import { Badge, Button, Field, Icon, TextArea } from "@marshal/ui";
import { Index, type JSX, Show } from "solid-js";
import type { MsgView } from "~/mock";

export interface PlanBlockProps {
  item: MsgView;
}

const SECTION_HEADING = "m-0 mb-1.5 text-body font-semibold";

function PlanEditForm(props: PlanBlockProps) {
  return (
    <form onSubmit={(e) => props.item.save?.(e)} class="flex flex-col gap-2 p-3.5">
      <Field label="Steps" hint="One step per line. The agent follows the plan as you save it.">
        <TextArea name="steps" rows={7} value={props.item.editText ?? ""} />
      </Field>
      <div class="flex gap-2">
        <Button variant="primary" type="submit">
          Save plan
        </Button>
        <Button onClick={() => props.item.cancel?.()}>Cancel</Button>
      </div>
    </form>
  );
}

function Section(props: { title: string; children: JSX.Element }) {
  return (
    <div>
      <h4 class={SECTION_HEADING}>{props.title}</h4>
      {props.children}
    </div>
  );
}

function PlanBody(props: PlanBlockProps) {
  return (
    <div class="grid gap-3.5 p-3.5">
      <Section title="Steps">
        <ol class="m-0 p-0 list-none flex flex-col gap-1">
          <Index each={props.item.steps ?? []}>
            {(step) => (
              <li class="flex gap-2">
                <span class="flex-none w-5 text-secondary">{step().n}</span>
                <span>{step().t}</span>
              </li>
            )}
          </Index>
        </ol>
      </Section>
      <Section title="Files to touch">
        <ul class="m-0 p-0 list-none flex flex-col gap-0.5">
          <Index each={props.item.files ?? []}>
            {(file) => <li class="font-mono text-small leading-5 text-secondary">{file()}</li>}
          </Index>
        </ul>
      </Section>
      <Section title="Risks">
        <ul class="m-0 pl-4.5 flex flex-col gap-0.5 text-secondary">
          <Index each={props.item.risks ?? []}>{(risk) => <li>{risk()}</li>}</Index>
        </ul>
      </Section>
      <Section title="Checks">
        <div class="flex flex-wrap gap-1.5">
          <Index each={props.item.checks ?? []}>
            {(check) => (
              <code class="py-px px-1.5 rounded-xs bg-surface-sunken font-mono text-caption leading-4.5">
                {check()}
              </code>
            )}
          </Index>
        </div>
      </Section>
    </div>
  );
}

function PlanActions(props: PlanBlockProps) {
  return (
    <div class="flex flex-wrap gap-2 py-2.5 px-3.5 border-t border-border bg-surface-sunken">
      <Button variant="primary" onClick={() => props.item.approve?.()}>
        Approve plan
      </Button>
      <Button onClick={() => props.item.edit?.()}>Edit plan</Button>
      <button
        type="button"
        onClick={() => props.item.reject?.()}
        class="h-8 px-3 rounded-sm border border-transparent bg-transparent text-status-danger-text font-medium hover:bg-status-danger-subtle"
      >
        Reject
      </button>
      <span class="flex-1" />
      <span class="self-center text-caption text-muted">Press A to approve</span>
    </div>
  );
}

/** A plan the agent proposes: steps, files, risks, and checks, with approve, edit, and reject. */
export function PlanBlock(props: PlanBlockProps) {
  return (
    <section
      aria-label="Plan"
      class="max-w-[72ch] border border-border-strong rounded-lg bg-surface overflow-hidden"
    >
      <div class="flex items-center gap-2 py-2.5 px-3.5 border-b border-border">
        <Icon name="list-checks" size={16} />
        <h3 class="m-0 flex-1 text-subtitle leading-5.5 font-semibold">Plan</h3>
        <Badge size={22} style={{ background: props.item.statusBg, color: props.item.statusColor }}>
          <Icon name={props.item.statusIcon ?? ""} size={12} />
          {props.item.statusLabel}
        </Badge>
      </div>
      <Show when={props.item.editing}>
        <PlanEditForm item={props.item} />
      </Show>
      <Show when={props.item.notEditing}>
        <PlanBody item={props.item} />
      </Show>
      <Show when={props.item.waiting}>
        <PlanActions item={props.item} />
      </Show>
    </section>
  );
}
