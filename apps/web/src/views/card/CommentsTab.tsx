import { Index, Show } from "solid-js";
import type { Card } from "~/mock";
import { CommentComposer } from "./CommentComposer";
import { CommentItem } from "./CommentItem";
import { commentViews } from "./comment-model";
import type { Panel } from "./panel-state";

export interface CommentsTabProps {
  card: Card;
  panel: Panel;
}

/** The Comments tab: the comment box, then the discussion, newest first. */
export function CommentsTab(props: CommentsTabProps) {
  const comments = () => commentViews(props.card);
  return (
    <div class="flex-1 min-h-0 overflow-auto py-3 px-4 flex flex-col gap-3.5">
      <CommentComposer cardId={props.card.id} panel={props.panel} />
      <Show when={comments().length === 0}>
        <p class="m-0 text-secondary">
          No comments yet. People and the card's agent can discuss the card here. The agent reads
          new comments at the start of its next turn.
        </p>
      </Show>
      <Index each={comments()}>
        {(comment) => <CommentItem cardId={props.card.id} comment={comment()} />}
      </Index>
    </div>
  );
}
