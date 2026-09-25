import { Scrim } from "@marshal/ui";
import { Show } from "solid-js";
import { M } from "~/mock";
import { CardDetail } from "~/views/card/CardDetail";
import { ResizeHandle } from "./ResizeHandle";
import { detailOpen, detailWidth, isDesktop, isPhone, isTablet } from "./shell-layout";

/** How the card panel sits: full screen on phones, over the content on tablets, beside it on desktop. */
function asideClass(): string {
  if (isPhone()) return "fixed inset-0 z-[250] bg-surface";
  if (isTablet()) {
    return "absolute top-0 right-0 bottom-0 w-[min(640px,94%)] z-detail bg-surface shadow-e2";
  }
  if (M.S.detailExpanded) return "relative flex-1 min-w-0 bg-surface";
  return "relative flex-none max-w-[60%] bg-surface";
}

/** Only the desktop side panel has its own width, set by dragging the handle. */
const asideWidth = (): { width: string } | undefined =>
  isDesktop() && !M.S.detailExpanded ? { width: `${detailWidth()}px` } : undefined;

/**
 * The open card: a side panel with a resize handle on desktop, an overlay with a backdrop
 * on tablets, and a full-screen page on phones. Renders nothing while no card is open.
 */
export function DetailPanel() {
  return (
    <Show when={detailOpen()}>
      <Show when={isTablet()}>
        <Scrim tone="side" class="z-[140]" onClick={() => M.closeCard()} />
      </Show>
      <Show when={isDesktop() && !M.S.detailExpanded}>
        <ResizeHandle />
      </Show>
      <aside aria-label="Card detail" class={asideClass()} style={asideWidth()}>
        <CardDetail />
      </aside>
    </Show>
  );
}
