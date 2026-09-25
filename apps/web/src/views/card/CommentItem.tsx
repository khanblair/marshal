import { Avatar, Icon, IconLabel } from "@marshal/ui";
import { Index, Show } from "solid-js";
import { M } from "~/mock";
import type { CommentView } from "./comment-model";

export interface CommentItemProps {
  cardId: number;
  comment: CommentView;
}

function Images(props: { images: CommentView["images"] }) {
  return (
    <div class="flex flex-wrap gap-2">
      <Index each={props.images}>
        {(image) => (
          <figure class="m-0 w-45 border border-border rounded-md overflow-hidden">
            <Show
              when={image().src}
              fallback={
                <div class="h-27.5 flex items-center justify-center bg-surface-sunken text-muted">
                  <Icon name="image" size={20} />
                </div>
              }
            >
              <img src={image().src} alt={image().name} class="block w-full h-27.5 object-cover" />
            </Show>
            <figcaption class="py-1 px-2 text-caption overflow-hidden text-ellipsis whitespace-nowrap">
              {image().name}
            </figcaption>
          </figure>
        )}
      </Index>
    </div>
  );
}

function Files(props: { files: CommentView["files"] }) {
  return (
    <div class="flex flex-wrap gap-1.5">
      <Index each={props.files}>
        {(file) => (
          <a
            href={file().href}
            target="_blank"
            rel="noopener"
            onClick={(e) => {
              if (file().placeholder) {
                e.preventDefault();
                M.toast(`Opening ${file().name}`);
              }
            }}
            class="inline-flex items-center gap-1.5 min-h-7.5 px-2.5 rounded-sm border border-border bg-surface text-small no-underline text-primary"
          >
            <Icon name={file().icon} size={14} />
            <span class={file().isLink ? "font-mono text-caption" : "font-sans text-small"}>
              {file().name}
            </span>
            <Show when={file().meta}>
              <span class="text-caption text-muted">{file().meta}</span>
            </Show>
          </a>
        )}
      </Index>
    </div>
  );
}

/** One comment: who wrote it, when, whether the agent read it, its text, and attachments. */
export function CommentItem(props: CommentItemProps) {
  return (
    <article class="flex gap-2.5">
      <Show
        when={props.comment.isAgent}
        fallback={<Avatar size={28} initials={props.comment.initials} aria-hidden="true" />}
      >
        <Avatar size={28} kind="agent" bordered aria-hidden="true" />
      </Show>
      <div class="flex-1 min-w-0 flex flex-col gap-1.5">
        <div class="flex flex-wrap items-baseline gap-x-2.5">
          <span class="font-semibold">{props.comment.name}</span>
          <span title={props.comment.full} class="text-caption text-muted">
            {props.comment.when}
          </span>
          <Show when={props.comment.readShow}>
            <IconLabel icon="check-check" size={12} gap={3} class="text-caption text-secondary">
              Agent read this
            </IconLabel>
          </Show>
        </div>
        <Show when={props.comment.text}>
          <div
            class={`max-w-[72ch] py-2 px-2.5 rounded-md border border-border whitespace-pre-wrap wrap-anywhere ${
              props.comment.isAgent ? "bg-surface" : "bg-surface-sunken"
            }`}
          >
            {props.comment.text}
          </div>
        </Show>
        <Show when={props.comment.images.length > 0}>
          <Images images={props.comment.images} />
        </Show>
        <Show when={props.comment.files.length > 0}>
          <Files files={props.comment.files} />
        </Show>
        <Show when={props.comment.mine}>
          <button
            type="button"
            onClick={() => M.deleteComment(props.cardId, props.comment.id)}
            class="self-start h-6 px-1.5 border-none rounded-xs bg-transparent text-secondary text-caption hover:bg-surface-hover"
          >
            Delete
          </button>
        </Show>
      </div>
    </article>
  );
}
