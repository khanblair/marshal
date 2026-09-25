import { Button, Icon } from "@marshal/ui";
import { createMemo, For, Show } from "solid-js";
import { type Card, M, type Project } from "~/mock";
import { cardLabel } from "~/mock/card-key";
import { canApproveHere, countLabel, nothingNeedsYouText, reasonButton, reasonIcon } from "./needs";

function NeedsRow(props: { card: Card; project: Project }) {
  const reason = () => props.card.reason || "";
  return (
    <div class="flex flex-wrap items-center gap-x-4 gap-y-2 py-2.5 border-t border-border">
      <button
        type="button"
        onClick={() => M.openCard(props.card.id)}
        class="flex-[1_1_260px] min-w-0 flex gap-2.5 items-start border-none bg-transparent p-0 text-left"
      >
        <Icon name={reasonIcon(reason())} size={16} class="mt-0.5 text-status-needs-you-solid" />
        <span class="flex flex-col gap-0.5 min-w-0">
          <span class="text-status-needs-you-text font-semibold">{reason()}</span>
          <span class="flex flex-wrap gap-x-2">
            <span>{props.card.title}</span>
            <span class="text-muted">{cardLabel(props.card)}</span>
          </span>
          <span class="flex flex-wrap gap-x-3 text-caption leading-4 text-secondary">
            <span>{props.project.name}</span>
            <span>{props.card.agent}</span>
            <span title={M.full(props.card.upd)}>
              Waiting since {M.rel(props.card.upd).toLowerCase()}
            </span>
          </span>
        </span>
      </button>
      <div class="flex gap-1.5">
        <Show when={canApproveHere(props.card)}>
          <Button variant="primary" class="hover:bg-ink!" onClick={() => M.approve(props.card.id)}>
            Approve
          </Button>
        </Show>
        <Button onClick={() => M.openCard(props.card.id)}>{reasonButton(reason())}</Button>
      </div>
    </div>
  );
}

function ProjectGroup(props: { project: Project; cards: readonly Card[] }) {
  return (
    <div class="flex flex-col">
      <button
        type="button"
        onClick={() => M.go("project", props.project.id)}
        class="self-start flex items-center gap-2 pt-2 pb-1.5 px-0 border-none bg-transparent font-semibold text-secondary hover:text-primary"
      >
        <Icon name="folder-git-2" size={14} />
        {props.project.name}
        <span class="font-normal">{countLabel(props.cards.length)}</span>
      </button>
      <For each={props.cards}>{(card) => <NeedsRow card={card} project={props.project} />}</For>
    </div>
  );
}

/** Home's "Needs you": the cards waiting on you, grouped by project, with the action each needs. */
export function NeedsSection() {
  const needs = createMemo(() => M.needs());
  const working = () => M.working().length;
  return (
    <section data-tour="needs" aria-labelledby="h-needs" class="flex flex-col gap-1">
      <div class="flex items-center gap-2 mb-1">
        <Icon name="st-needs" size={20} class="text-status-needs-you-solid" />
        <h2 id="h-needs" class="m-0 text-view-title leading-7 font-bold">
          Needs you
        </h2>
        <Show when={needs().length > 0}>
          <span class="min-w-6 h-6 px-2 rounded-xs bg-status-needs-you-subtle text-status-needs-you-text font-bold leading-6 text-center">
            {needs().length}
          </span>
        </Show>
      </div>
      <Show when={needs().length === 0}>
        <p class="m-0 text-secondary">{nothingNeedsYouText(working())}</p>
      </Show>
      <For each={M.S.projects}>
        {(project) => {
          const cards = createMemo(() => needs().filter((c) => c.p === project.id));
          return (
            <Show when={cards().length > 0}>
              <ProjectGroup project={project} cards={cards()} />
            </Show>
          );
        }}
      </For>
    </section>
  );
}
