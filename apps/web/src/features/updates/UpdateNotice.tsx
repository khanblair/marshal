import { Button, Icon, IconButton } from "@marshal/ui";
import { createSignal, onCleanup, onMount, Show } from "solid-js";
import { M } from "~/mock";
import { updates } from "./updates";

export function openUpdates() {
  M.set({ settingsSection: "updates", sideOpen: false, menu: null });
  M.go("settings");
}

/** A quiet prompt; checking never downloads or takes focus from ongoing work. */
const AUTO_CHECK_MS = 14_400_000;
const STATUS_POLL_MS = 1000;

export function UpdateNotice() {
  const [dismissed, dismiss] = createSignal("");
  onMount(() => {
    void updates.initialize();
    const timer = setInterval(() => void updates.initialize(), AUTO_CHECK_MS);
    const progress = setInterval(() => {
      if (["checking", "downloading", "installing", "restarting"].includes(updates.state.status))
        void updates.refresh();
    }, STATUS_POLL_MS);
    onCleanup(() => {
      clearInterval(timer);
      clearInterval(progress);
    });
  });
  const visible = () =>
    ["available", "installed"].includes(updates.state.status) &&
    dismissed() !== `${updates.state.status}:${updates.state.version}`;
  return (
    <Show when={visible()}>
      <div
        role="status"
        class="flex flex-wrap items-center gap-2 border-b border-border bg-surface px-4 py-2"
      >
        <Icon name="download" size={16} />
        <span class="flex-1 min-w-0 text-small">
          Marshal {updates.state.version}{" "}
          {updates.state.status === "installed" ? "is ready to restart." : "is available."}
        </span>
        <Button size={28} onClick={openUpdates}>
          View update
        </Button>
        <IconButton
          label="Remind me later"
          icon="x"
          onClick={() => dismiss(`${updates.state.status}:${updates.state.version}`)}
        />
      </div>
    </Show>
  );
}
