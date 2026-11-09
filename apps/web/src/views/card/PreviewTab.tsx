import {
  type PreviewConnection,
  type PreviewShot,
  type PreviewShotKind,
  PreviewShotKindAfter,
  PreviewShotKindBefore,
} from "@marshal/protocol";
import { Button, Icon } from "@marshal/ui";
import { createEffect, createSignal, Index, onCleanup, Show } from "solid-js";
import { type Card, M } from "~/mock";
import { isLoopbackHttpUrl } from "./preview-link";
import {
  DARK_PREVIEW_CARD_ID,
  previewOf,
  previewState,
  previewStatus,
  previewUrl,
} from "./preview-model";

export interface PreviewTabProps {
  card: Card;
}

/** The before and after halves, in the order the pair is read. */
const KINDS: readonly PreviewShotKind[] = [PreviewShotKindBefore, PreviewShotKindAfter];
const LABELS: Record<PreviewShotKind, string> = { before: "Before", after: "After" };

/** Card #118 previews a dark page, whatever the theme; every other card follows the theme. */
function tones(card: Card) {
  const dark = card.id === DARK_PREVIEW_CARD_ID;
  return {
    side: dark ? "bg-card-preview-dark-side" : "bg-surface-sunken",
    main: dark ? "bg-card-preview-dark-main" : "bg-surface",
    block: dark ? "bg-card-preview-dark-block" : "bg-border-strong",
    blockBorder: dark ? "border-card-preview-dark-block" : "border-border-strong",
    beforeBg: dark ? "bg-white" : "bg-surface-sunken",
    afterBg: dark ? "bg-card-preview-dark-side" : "bg-surface-sunken",
  };
}

/** A drawn stand-in for the running app: a sidebar with three lines, a heading, and two boxes. */
function PagePreview(props: { card: Card }) {
  const look = () => tones(props.card);
  return (
    <div class="border border-border rounded-md overflow-hidden bg-surface">
      <div class="flex min-h-75">
        <div class={`w-[30%] p-4 flex flex-col gap-2.5 border-r border-border ${look().side}`}>
          <div class={`h-3 w-[70%] rounded-xs ${look().block}`} />
          <div class={`h-3 w-[55%] rounded-xs ${look().block}`} />
          <div class={`h-3 w-[62%] rounded-xs ${look().block}`} />
        </div>
        <div class={`flex-1 p-4 flex flex-col gap-3 ${look().main}`}>
          <div class={`h-4.5 w-[40%] rounded-xs ${look().block}`} />
          <div class={`h-16 rounded-sm border ${look().blockBorder}`} />
          <div class={`h-16 rounded-sm border ${look().blockBorder}`} />
        </div>
      </div>
      <div class="py-1.5 px-2.5 border-t border-border text-caption text-secondary">
        Live preview from the project folder, in this card's browser profile
      </div>
    </div>
  );
}

/**
 * One screenshot the daemon took. The image needs the token, so a bare `img` cannot fetch it: the
 * bytes come through the client and are drawn from an object URL, which is revoked when the shot is
 * replaced or the tab closes (the way an avatar is, `sync/avatar.ts`).
 */
function ShotImage(props: { shot: PreviewShot; label: string }) {
  const [src, setSrc] = createSignal("");
  let objectUrl: string | null = null;
  const release = (): void => {
    if (objectUrl) URL.revokeObjectURL(objectUrl);
    objectUrl = null;
  };
  createEffect(() => {
    const url = props.shot.url;
    let newest = true;
    onCleanup(() => {
      newest = false;
    });
    void M.previewShotImage(url).then((bytes) => {
      // A shot replaced while this one was being fetched must not draw over the newer one.
      if (!newest) return;
      release();
      if (!bytes) {
        setSrc("");
        return;
      }
      objectUrl = URL.createObjectURL(bytes);
      setSrc(objectUrl);
    });
  });
  onCleanup(release);
  return (
    <div class="h-30 bg-surface-sunken flex items-center justify-center overflow-hidden">
      <Show when={src()} fallback={<Icon name="spinner" size={16} />}>
        <img src={src()} alt={`${props.label} screenshot`} class="w-full h-full object-contain" />
      </Show>
    </div>
  );
}

/** The drawn stand-in for a half of the pair the daemon has not taken yet. */
function ShotPlaceholder(props: { card: Card; kind: PreviewShotKind }) {
  const look = () => tones(props.card);
  const bg = () => (props.kind === PreviewShotKindBefore ? look().beforeBg : look().afterBg);
  const fill = () =>
    props.kind === PreviewShotKindBefore
      ? "bg-card-preview-light-block"
      : "bg-card-preview-dark-block";
  const line = () =>
    props.kind === PreviewShotKindBefore
      ? "border-card-preview-light-block"
      : "border-card-preview-dark-block";
  return (
    <div class={`h-30 flex flex-col gap-2 p-3 ${bg()}`}>
      <div class={`h-2.5 w-1/2 rounded-xs ${fill()}`} />
      <div class={`h-10 rounded-xs border ${line()}`} />
    </div>
  );
}

/** The before and after screenshots of the running preview, with a way to take either that is missing. */
function Screenshots(props: { card: Card }) {
  const shotOf = (kind: PreviewShotKind): PreviewShot | undefined =>
    previewOf(props.card.id)?.shots.find((shot) => shot.kind === kind);
  return (
    <>
      <h3 class="m-0 mt-1 text-subtitle leading-5.5 font-semibold">Screenshots</h3>
      <div class="grid gap-2.5 grid-cols-[repeat(auto-fit,minmax(180px,1fr))]">
        <Index each={KINDS}>
          {(kind) => (
            <figure class="m-0 border border-border rounded-md overflow-hidden">
              <Show
                when={shotOf(kind())}
                fallback={<ShotPlaceholder card={props.card} kind={kind()} />}
              >
                {(shot) => <ShotImage shot={shot()} label={LABELS[kind()]} />}
              </Show>
              <figcaption class="flex items-center gap-2 py-1.5 px-2.5 text-caption border-t border-border">
                <span class="flex-1">{LABELS[kind()]}</span>
                <Show when={!shotOf(kind())}>
                  <Button
                    variant="secondary"
                    class="px-2! py-0.5! text-caption!"
                    onClick={() => void M.takePreviewShot(props.card.id, kind())}
                  >
                    Take screenshot
                  </Button>
                </Show>
              </figcaption>
            </figure>
          )}
        </Index>
      </div>
    </>
  );
}

function NoPreview() {
  return (
    <div class="py-6 px-2 flex flex-col items-center gap-3 text-center">
      <p class="m-0 max-w-[44ch] text-secondary">
        api-gateway has no dev command that serves a web page, so there is nothing to preview. Add a
        dev command in the project settings to preview it here.
      </p>
      <Button
        class="font-medium"
        onClick={() => M.toast("Open project settings to add a dev command")}
      >
        Add dev command
      </Button>
    </div>
  );
}

/** The Preview tab: start the card's dev server and see the app it serves. */
export function PreviewTab(props: PreviewTabProps) {
  const state = () => previewState(props.card.id);
  const stopped = () => state() === "stopped";
  const [target, setTarget] = createSignal("");
  const [connecting, setConnecting] = createSignal(false);
  const [connectError, setConnectError] = createSignal("");
  const [connected, setConnected] = createSignal<PreviewConnection>();
  const [autoAttempted, setAutoAttempted] = createSignal("");
  let requestId = 0;
  let disposed = false;
  onCleanup(() => {
    disposed = true;
    requestId += 1;
  });
  const daemonPreview = () => M.previewOnDaemon();
  const targetUrl = () => target() || M.previewTarget(props.card.id) || previewUrl(props.card.id);
  const savedTargetUrl = () => M.previewTarget(props.card.id) || previewUrl(props.card.id);
  const connection = () => {
    const current = connected();
    return current?.sourceUrl === targetUrl() && Date.parse(current.expiresAt) > Date.now()
      ? current
      : undefined;
  };
  const connect = async (url = targetUrl()) => {
    if (!url) return;
    if (!isLoopbackHttpUrl(url)) {
      setConnectError("Enter a local HTTP address such as http://localhost:3000.");
      return;
    }
    const currentRequest = ++requestId;
    setConnecting(true);
    setConnectError("");
    M.setPreviewTarget(props.card.id, url);
    const result = await M.connectPreview(props.card.id, url);
    if (disposed || currentRequest !== requestId) return;
    setConnected(result ?? undefined);
    setConnecting(false);
    if (!result)
      setConnectError("Could not connect to this preview. Check the address and try again.");
  };
  createEffect(() => {
    const url = savedTargetUrl();
    if (
      daemonPreview() &&
      url &&
      (state() === "running" || M.previewTarget(props.card.id)) &&
      autoAttempted() !== url
    ) {
      setAutoAttempted(url);
      void connect(url);
    }
  });
  return (
    <div class="flex-1 min-h-0 overflow-auto py-3 px-4 flex flex-col gap-3">
      <Show when={props.card.p === "api" && !daemonPreview()}>
        <NoPreview />
      </Show>
      <Show when={props.card.p !== "api" || daemonPreview()}>
        <div class="flex items-center gap-2">
          <div class="flex-1 min-w-0 flex items-center gap-2 h-8 px-2.5 rounded-sm border border-border-strong bg-surface-sunken">
            <Show when={state() === "starting"}>
              <Icon name="spinner" size={14} />
            </Show>
            <Show when={state() === "running"}>
              <span class="inline-flex text-status-working-solid">
                <Icon name="circle-check" size={14} />
              </span>
            </Show>
            <Show
              when={previewUrl(props.card.id)}
              fallback={<span class="text-small text-muted">No server running</span>}
            >
              <span class="font-mono text-small overflow-hidden text-ellipsis whitespace-nowrap">
                {previewUrl(props.card.id)}
              </span>
            </Show>
          </div>
          <Button
            variant={stopped() ? "primary" : "secondary"}
            class="font-semibold!"
            onClick={() =>
              stopped() ? void M.startPreview(props.card.id) : void M.stopPreview(props.card.id)
            }
          >
            {stopped() ? "Start preview" : "Stop preview"}
          </Button>
        </div>
        <span class="text-small text-secondary">{previewStatus(props.card.id)}</span>
        <Show when={state() === "running"}>
          <Show when={!daemonPreview()}>
            <PagePreview card={props.card} />
          </Show>
          <Show when={props.card.p !== "api"}>
            <Screenshots card={props.card} />
          </Show>
        </Show>
      </Show>
      <Show when={daemonPreview()}>
        <div class="flex flex-wrap items-center gap-2">
          <label class="sr-only" for={`preview-url-${props.card.id}`}>
            Preview address
          </label>
          <input
            id={`preview-url-${props.card.id}`}
            class="h-8 phone:h-(--control-h) min-w-0 flex-1 rounded-sm border border-border-strong bg-surface px-2.5 text-small font-mono"
            type="url"
            placeholder="http://localhost:3000"
            value={targetUrl()}
            disabled={connecting()}
            onInput={(event) => setTarget(event.currentTarget.value)}
          />
          <Button variant="secondary" disabled={connecting()} onClick={() => void connect()}>
            {connecting() ? "Connecting…" : connection() ? "Refresh connection" : "Connect"}
          </Button>
        </div>
        <Show when={connectError()}>
          <div role="alert" class="flex items-center gap-2 text-small text-status-danger-solid">
            <span class="flex-1">{connectError()}</span>
            <Button variant="secondary" onClick={() => void connect()}>
              Retry
            </Button>
          </div>
        </Show>
        <Show
          when={connection()}
          fallback={
            <div
              role="status"
              class="flex min-h-75 items-center justify-center rounded-md border border-border bg-surface-sunken text-small text-secondary"
            >
              {connecting()
                ? "Connecting to the running preview…"
                : "Connect a running local preview to view it here."}
            </div>
          }
        >
          {(connected) => (
            <iframe
              title="Live project preview"
              src={connected().url}
              sandbox="allow-scripts allow-same-origin allow-forms"
              referrerPolicy="no-referrer"
              class="w-full min-h-75 flex-1 rounded-md border border-border bg-surface"
            />
          )}
        </Show>
      </Show>
    </div>
  );
}
