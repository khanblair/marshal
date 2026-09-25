import { cx, Icon, IconButton } from "@marshal/ui";
import { Index, Show } from "solid-js";
import { NoticeAction } from "./NoticeAction";
import { NoticeRow } from "./NoticeRow";
import type { NoticeModel, NoticeTone } from "./notice-list";

const ICON_PX = 16;
const DISMISS_ICON_PX = 14;

const ICON_COLORS: Record<NoticeTone, string> = {
  needs: "text-status-needs-you-solid",
  secondary: "text-secondary",
  danger: "text-status-danger-solid",
};

/** One notice: its icon, title, time, and optional dismiss button, then rows and actions. */
export function NoticeCard(props: { notice: NoticeModel }) {
  return (
    <article class="flex-none border border-border rounded-md bg-surface overflow-hidden">
      <div class="flex gap-2.5 p-3">
        <span class={cx("inline-flex mt-0.5", ICON_COLORS[props.notice.tone])}>
          <Icon name={props.notice.icon} size={ICON_PX} />
        </span>
        <div class="flex-1 min-w-0 flex flex-col gap-0.5">
          <div class="font-semibold">{props.notice.title}</div>
          <Show when={props.notice.sub}>
            <div class="text-small leading-4.5 text-secondary">{props.notice.sub}</div>
          </Show>
          <span title={props.notice.full} class="text-caption leading-4 text-muted">
            {props.notice.when}
          </span>
        </div>
        <Show when={props.notice.dismiss}>
          <IconButton
            label="Dismiss notice"
            icon="x"
            iconSize={DISMISS_ICON_PX}
            onClick={() => props.notice.dismiss?.()}
          />
        </Show>
      </div>
      <Show when={props.notice.rows.length > 0}>
        <div class="border-t border-border">
          <Index each={props.notice.rows}>{(row) => <NoticeRow row={row()} />}</Index>
        </div>
      </Show>
      <Show when={props.notice.actions.length > 0}>
        <div class="flex flex-wrap gap-2 py-2 px-3 bg-surface-sunken">
          <Index each={props.notice.actions}>
            {(action) => <NoticeAction action={action()} size="card" />}
          </Index>
        </div>
      </Show>
    </article>
  );
}
