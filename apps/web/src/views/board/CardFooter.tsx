import { Avatar, CiStatus, IconLabel } from "@marshal/ui";
import { Index, Show } from "solid-js";
import type { CardView } from "~/mock";

export interface CardFooterProps {
  c: CardView;
}

function Members(props: CardFooterProps) {
  return (
    // biome-ignore lint/a11y/useAriaPropsSupportedByRole: the design labels this group of avatars
    <span aria-label="Members" class="ml-auto flex">
      <Index each={props.c.avatars}>
        {(a) => (
          <Show
            when={a().isAgent}
            fallback={
              <Avatar ring initials={a().initials} title={a().title} class="-ml-1 leading-2.5" />
            }
          >
            <Avatar kind="agent" ring title={a().title} class="-ml-1" />
          </Show>
        )}
      </Index>
    </span>
  );
}

function CardMeta(props: CardFooterProps) {
  return (
    <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-caption leading-4 text-secondary">
      <Show when={props.c.hasCi}>
        <CiStatus status={props.c.ciLabel.toLowerCase()} title={props.c.ciTip}>
          {props.c.ciLabel}
        </CiStatus>
      </Show>
      <Show when={props.c.hasCost}>
        <IconLabel icon="circle-dollar-sign" gap={3} title="Cost of this card">
          {props.c.cost}
        </IconLabel>
      </Show>
      <Show when={props.c.hasCl}>
        <IconLabel icon="square-check" gap={3} title={props.c.clTip}>
          {props.c.clLabel}
        </IconLabel>
      </Show>
      <Show when={props.c.hasComments}>
        <IconLabel icon="message-square" gap={3} title={props.c.commentsTip}>
          {props.c.commentsN}
        </IconLabel>
      </Show>
      <Show when={props.c.hasAttach}>
        <IconLabel icon="paperclip" gap={3} title={props.c.attachTip}>
          {props.c.attachN}
        </IconLabel>
      </Show>
      <Show when={props.c.pinned}>
        <IconLabel icon="pin" gap={3} title="Pinned: this card never sleeps">
          Pinned
        </IconLabel>
      </Show>
      <Show when={props.c.hasAvatars}>
        <Members c={props.c} />
      </Show>
    </div>
  );
}

/** The card's bottom block: branch, then CI, cost, checklist, comments, attachments, and members. */
export function CardFooter(props: CardFooterProps) {
  return (
    <div class="flex flex-col gap-1.5">
      <Show when={props.c.hasBranch}>
        <IconLabel icon="git-branch" gap={6} class="min-w-0 text-secondary">
          <span class="flex-1 min-w-0 truncate font-mono text-caption leading-4">
            {props.c.branch}
          </span>
        </IconLabel>
      </Show>
      <CardMeta c={props.c} />
    </div>
  );
}
