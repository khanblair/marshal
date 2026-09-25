import { cx, IconButton, Scrim } from "@marshal/ui";
import { createMemo, Index, Show } from "solid-js";
import { M } from "~/mock";
import { NoticeCard } from "./NoticeCard";
import { buildNotices } from "./notice-list";

const closeNotices = (): void => M.set({ noticesOpen: false });

/**
 * The notices panel: a dropdown card on tablet and desktop, a full page under the top bar on
 * phones. Renders nothing while `M.S.noticesOpen` is off.
 */
export function NoticesPanel() {
  const notices = createMemo(() => buildNotices(M, M.mobile));
  return (
    <Show when={M.S.noticesOpen}>
      <Scrim tone="clear" class="z-[99]" onClick={closeNotices} />
      <section
        aria-label="Notices"
        class={cx(
          "absolute z-menu flex flex-col",
          M.mobile
            ? "left-0 right-0 top-12 bottom-0 bg-canvas"
            : "right-2 top-13 w-[min(420px,calc(100%-16px))] max-h-[calc(100%-60px)] rounded-lg border border-border bg-surface-raised shadow-e1",
        )}
      >
        <div class="flex items-center gap-2 py-2.5 px-4 border-b border-border">
          <h2 class="m-0 flex-1 text-subtitle leading-5.5">Notices</h2>
          <IconButton label="Close notices" icon="x" tone="default" onClick={closeNotices} />
        </div>
        <div class="flex-1 min-h-0 flex flex-col gap-2 p-3 overflow-auto">
          <Show when={notices().length === 0}>
            <p class="my-4 mx-0 text-center text-secondary">
              No notices. Sleep warnings, CI failures, and cost warnings show here.
            </p>
          </Show>
          <Index each={notices()}>{(notice) => <NoticeCard notice={notice()} />}</Index>
        </div>
      </section>
    </Show>
  );
}
