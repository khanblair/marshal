import { Button, Icon, Input, Tag } from "@marshal/ui";
import { Index, Show } from "solid-js";
import type { CardKey } from "~/mock/card-key";
import { createComposerActions } from "./comment-composer-actions";
import type { Panel } from "./panel-state";

export interface CommentComposerProps {
  cardId: CardKey;
  panel: Panel;
}

const ATTACH_LABEL =
  "inline-flex items-center gap-1.5 h-8 px-2 rounded-sm text-secondary text-small cursor-pointer hover:bg-surface-hover";
const HIDDEN_INPUT = "absolute w-px h-px opacity-0";

const attachmentIcon = (kind: string): string => {
  if (kind === "image") return "image";
  return kind === "link" ? "link" : "file-text";
};

/** The new-comment box: text, attachments (files, images, links), and Post comment. */
export function CommentComposer(props: CommentComposerProps) {
  let linkInput: HTMLInputElement | undefined;
  const { state, empty, post, addLink, onFiles, toggleLink, onLinkKey } = createComposerActions(
    props,
    () => linkInput,
  );
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        post();
      }}
      class="flex flex-col gap-2 p-2.5 rounded-lg border border-border-strong bg-surface"
    >
      <textarea
        name="body"
        value={state().cDraft}
        onInput={(e) => props.panel.set({ cDraft: e.currentTarget.value })}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            post();
          }
        }}
        rows={3}
        placeholder="Write a comment. Mention @agent to ask the card's agent."
        aria-label="Write a comment"
        class="border-none outline-none bg-transparent resize-y p-0.5"
      />
      <Show when={state().pending.length > 0}>
        <div class="flex flex-wrap gap-1.5">
          <Index each={state().pending}>
            {(att, index) => (
              <Tag
                size={28}
                icon={attachmentIcon(att().kind)}
                removeLabel={`Remove ${att().name}`}
                onRemove={() =>
                  props.panel.set({ pending: state().pending.filter((_, j) => j !== index) })
                }
              >
                <span class="max-w-45 overflow-hidden text-ellipsis whitespace-nowrap">
                  {att().name}
                </span>
              </Tag>
            )}
          </Index>
        </div>
      </Show>
      <Show when={state().linkOpen}>
        <div class="flex gap-1.5">
          <Input
            ref={linkInput}
            mono
            placeholder="Paste a link"
            aria-label="Link"
            onKeyDown={onLinkKey}
            class="flex-1 min-w-0"
          />
          <Button class="text-small" onClick={addLink}>
            Add link
          </Button>
        </div>
      </Show>
      <div class="flex flex-wrap items-center gap-1">
        <label title="Attach a file" class={ATTACH_LABEL}>
          <Icon name="paperclip" size={14} />
          Attach file
          <input type="file" multiple onChange={onFiles} class={HIDDEN_INPUT} />
        </label>
        <label title="Add an image" class={ATTACH_LABEL}>
          <Icon name="image" size={14} />
          Add image
          <input type="file" accept="image/*" multiple onChange={onFiles} class={HIDDEN_INPUT} />
        </label>
        <button
          type="button"
          onClick={toggleLink}
          class="inline-flex items-center gap-1.5 h-8 px-2 border-none rounded-sm bg-transparent text-secondary text-small hover:bg-surface-hover"
        >
          <Icon name="link" size={14} />
          Add link
        </button>
        <span class="flex-1" />
        <Button variant="primary" type="submit" disabled={empty()}>
          Post comment
        </Button>
      </div>
    </form>
  );
}
