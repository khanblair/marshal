import { Button, IconButton, Select } from "@marshal/ui";
import { createSignal, onCleanup, onMount, Show } from "solid-js";

const VIEWPORTS: readonly { value: string; label: string; width?: number; height?: number }[] = [
  { value: "responsive", label: "Responsive" },
  { value: "wide", label: "1280 × 800", width: 1280, height: 800 },
  { value: "desktop", label: "1024 × 768", width: 1024, height: 768 },
  { value: "mobile", label: "Mobile · 375 × 812", width: 375, height: 812 },
  { value: "tablet", label: "Tablet · 768 × 1024", width: 768, height: 1024 },
];

interface LivePreviewProps {
  url: string;
  external: boolean;
  stopping: boolean;
  onStop: () => void;
  onDisconnect: () => void;
}

/** Keep the same iframe when changing the viewport or expanding inside the app. */
export function LivePreview(props: LivePreviewProps) {
  const [expanded, setExpanded] = createSignal(false);
  const [selected, setSelected] = createSignal("responsive");
  const [available, setAvailable] = createSignal({ width: 0, height: 0 });
  const viewport = () => VIEWPORTS.find((size) => size.value === selected());
  const viewportWidth = () => viewport()?.width ?? 0;
  const viewportHeight = () => viewport()?.height ?? 0;
  const scale = () => {
    const { width, height } = available();
    const targetWidth = viewportWidth();
    const targetHeight = viewportHeight();
    return targetWidth && targetHeight && width && height
      ? Math.min(1, width / targetWidth, height / targetHeight)
      : 1;
  };
  let panel!: HTMLDialogElement;
  let stage!: HTMLDivElement;
  let expandButton: HTMLButtonElement | undefined;
  onMount(() => {
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry) setAvailable(entry.contentRect);
    });
    observer.observe(stage);
    onCleanup(() => observer.disconnect());
  });

  function toggle() {
    if (expanded()) {
      panel.close();
      panel.setAttribute("open", "");
    } else {
      // A non-modal open dialog must be closed before it can enter the top layer.
      panel.removeAttribute("open");
      panel.showModal();
    }
    setExpanded(!expanded());
    expandButton?.focus();
  }

  return (
    <dialog
      ref={panel}
      open
      aria-label="Project preview"
      class="flex flex-col gap-2 border-0 bg-surface text-primary"
      classList={{
        "static m-0 w-full flex-1 min-h-0 overflow-visible p-0": !expanded(),
        "fixed inset-0 m-0 h-full w-full max-h-none max-w-none px-3 pt-[calc(12px+var(--safe-top))] pb-[calc(12px+var(--safe-bottom))]":
          expanded(),
      }}
      onCancel={(event) => {
        event.preventDefault();
        toggle();
      }}
      onKeyDown={(event) => {
        if (!expanded() || event.key !== "Escape") return;
        event.stopPropagation();
        if (!event.defaultPrevented) {
          event.preventDefault();
          toggle();
        }
      }}
    >
      <div class="flex flex-wrap items-center gap-2">
        <Select
          aria-label="Preview viewport"
          options={VIEWPORTS}
          value={selected()}
          onChange={(event) => setSelected(event.value)}
        />
        <Show when={props.external}>
          <Button variant="secondary" onClick={props.onDisconnect}>
            Disconnect
          </Button>
        </Show>
        <Show when={expanded() && !props.external}>
          <Button variant="secondary" disabled={props.stopping} onClick={props.onStop}>
            {props.stopping ? "Stopping…" : "Stop preview server"}
          </Button>
        </Show>
        <span class="flex-1" />
        <IconButton
          ref={expandButton}
          label={expanded() ? "Exit expanded preview" : "Expand preview"}
          title={expanded() ? "Exit expanded preview" : "Expand preview"}
          icon={expanded() ? "minimize-2" : "maximize-2"}
          variant="outline"
          size={32}
          onClick={toggle}
        />
      </div>
      <div
        ref={stage}
        class="min-w-0 flex-1 overflow-hidden rounded-md border border-border bg-surface-sunken grid place-items-center"
        classList={{ "min-h-0": expanded(), "min-h-75": !expanded() }}
      >
        <div
          class="relative overflow-hidden"
          style={{
            width: viewportWidth() ? `${viewportWidth() * scale()}px` : "100%",
            height: viewportHeight() ? `${viewportHeight() * scale()}px` : "100%",
          }}
        >
          <iframe
            title="Live project preview"
            src={props.url}
            sandbox="allow-scripts allow-same-origin allow-forms"
            referrerPolicy="no-referrer"
            class="absolute inset-0 block border-0 origin-top-left bg-surface"
            style={{
              width: viewportWidth() ? `${viewportWidth()}px` : "100%",
              height: viewportHeight() ? `${viewportHeight()}px` : "100%",
              transform: `scale(${scale()})`,
            }}
          />
        </div>
      </div>
    </dialog>
  );
}
