import { Button, Callout, Checkbox, Dialog, Field, Input, Select, TextArea } from "@marshal/ui";
import { batch, For, Show } from "solid-js";
import { M, type NewCardDraft } from "~/mock";
import { currentProject } from "../shell-layout";
import { DialogHeader } from "./DialogHeader";

const TEMPLATES = ["Blank", "Bug fix", "New endpoint", "Refactor", "Plan first"];
const DEFAULT_TEMPLATE = "Blank";
const DEFAULT_ROLE = "Worker";
const DEFAULT_AGENT = "Claude Code";

const closeNewCard = (): void => M.set({ newCard: null });

function openDuplicate(id: number): void {
  batch(() => {
    M.set({ newCard: null });
    M.openCard(id);
  });
}

/** Cards in this project whose titles share words with the draft's title. */
function Duplicates(props: { title: string }) {
  const dupes = () => M.dupes(props.title || "");
  return (
    <Show when={dupes().length > 0}>
      <Callout icon="copy">
        <div class="flex flex-col gap-0.5">
          <strong>This looks like an existing card</strong>
          <For each={dupes()}>
            {(card) => (
              <button
                type="button"
                onClick={() => openDuplicate(card.id)}
                class="border-none bg-transparent p-0 text-left text-inherit underline"
              >
                #{card.id} {card.title}
              </button>
            )}
          </For>
        </div>
      </Callout>
    </Show>
  );
}

/** Template, role, and agent, side by side while they fit. */
function CardOptions(props: { draft: NewCardDraft }) {
  return (
    <div class="grid grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-3">
      <Field label="Template">
        <Select
          options={TEMPLATES}
          value={props.draft.template || DEFAULT_TEMPLATE}
          onChange={(e) => {
            props.draft.template = e.currentTarget.value;
          }}
        />
      </Field>
      <Field label="Role">
        <Select
          options={M.ROLE_NAMES}
          value={props.draft.role || DEFAULT_ROLE}
          onChange={(e) => {
            props.draft.role = e.currentTarget.value;
          }}
        />
      </Field>
      <Field label="Agent">
        <Select
          options={Object.keys(M.AGENTS)}
          value={props.draft.agent || DEFAULT_AGENT}
          onChange={(e) => {
            props.draft.agent = e.currentTarget.value;
          }}
        />
      </Field>
    </div>
  );
}

function NewCardForm(props: { draft: NewCardDraft }) {
  const empty = () => !(props.draft.title || "").trim();
  return (
    <Dialog
      width={560}
      phone={M.mobile}
      aria-labelledby="nc-title"
      onClose={closeNewCard}
      onSubmit={(e) => {
        e.preventDefault();
        M.createCard();
      }}
    >
      <DialogHeader
        id="nc-title"
        title={`New card in ${currentProject().name}`}
        onClose={closeNewCard}
      />
      <Field label="Title">
        <Input
          data-autofocus
          value={props.draft.title}
          onInput={(e) => {
            props.draft.title = e.currentTarget.value;
          }}
          placeholder="Fix token refresh on login"
        />
      </Field>
      <Duplicates title={props.draft.title} />
      <Field label="Description" hint="This becomes the first message in the card's session.">
        <TextArea
          value={props.draft.body}
          onInput={(e) => {
            props.draft.body = e.currentTarget.value;
          }}
          rows={3}
          placeholder="What should the agent do, and what does done mean?"
        />
      </Field>
      <CardOptions draft={props.draft} />
      <label class="flex items-center gap-2 cursor-pointer">
        <Checkbox
          checked={props.draft.start}
          onChange={() => {
            props.draft.start = !props.draft.start;
          }}
        />
        <span>Start the card right away</span>
      </label>
      <div class="flex justify-end gap-2">
        <Button class="hover:bg-surface!" onClick={closeNewCard}>
          Cancel
        </Button>
        <Button variant="primary" type="submit" class="hover:bg-ink!" disabled={empty()}>
          Create card
        </Button>
      </div>
    </Dialog>
  );
}

/**
 * The New card form (`M.newCard()` opens it). The title has focus when it opens; a title that
 * looks like an open card shows a warning. Renders nothing while `M.S.newCard` is empty.
 */
export function NewCardDialog() {
  return <Show when={M.S.newCard}>{(draft) => <NewCardForm draft={draft()} />}</Show>;
}
