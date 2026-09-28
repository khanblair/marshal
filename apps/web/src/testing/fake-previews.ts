/**
 * A card's live preview in the fake daemon (section S13, docs/backend-checklist.md B6.6 and B6.7,
 * docs/architecture.md 11.2). One dev server per card, on a port of its own, with the before and
 * after screenshots taken of it. The daemon owns all of it, and these routes answer the way its own
 * handler does, sentence for sentence, so a test drives the same answers a person sees.
 *
 * There is no dev server behind this one to run: `start` answers the preview as it is once the
 * server answered (`running`, with its own address and port), and `stop` answers it stopped. The
 * daemon's own `start` answers `starting` first and only a real process settles it into `running`,
 * so a test that wants to draw that in-between frame emits it as a `preview.state_changed` event,
 * which is the same event the daemon publishes when it settles.
 *
 * Every change is published as `preview.state_changed` on the card's own topic, exactly as the
 * daemon's own `publish` does, because the card's tab is drawn from the event as much as from the
 * answer. The image route answers real PNG bytes, so a screen that fetches a shot has bytes to show.
 */
import {
  EventTypePreviewStateChanged,
  type Preview,
  type PreviewShot,
  type PreviewShotKind,
  PreviewShotKindBefore,
  PreviewShotKindValues,
  PreviewShotOutcomeSkipped,
  PreviewShotOutcomeTaken,
  type PreviewShotRequest,
  type PreviewShotResult,
  type PreviewSnapshot,
  PreviewStateRunning,
  PreviewStateStarting,
  PreviewStateStopped,
  type Card as WireCard,
} from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { STATUS } from "./fake-card-shared";

/** The daemon's own sentences, word for word (daemon/internal/preview/service.go). */
const NO_COMMAND =
  "This project has no dev command, so Marshal does not know how to start it. Add one in project settings.";
const NOT_RUNNING =
  "This card's preview is not running, so there is nothing to capture. Start the preview first.";
const NO_BROWSER =
  "Marshal found no Chrome or Edge on this machine, so the screenshot check was skipped. Install Chrome or Edge to take screenshots.";
const TAKEN = "The screenshot was taken.";
const NO_SUCH_KIND = "Marshal does not know that screenshot kind. Use before or after.";

/** Where the ports a card's dev server is given start, so two cards never share one. */
const FIRST_PORT = 5100;

/** A one-pixel PNG, base64: the bytes a shot's image route answers with. Nothing in a test decodes it. */
const SHOT_PNG = Uint8Array.from(
  atob(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
  ),
  (char) => char.charCodeAt(0),
);

/** Sends an event on a topic, as the daemon does after a change. */
type Publish = (topic: string, type: string, data: unknown) => void;

/** Every card's preview, and the stream a change is announced on, as the fake daemon holds them. */
export interface PreviewStore {
  /** Every card's preview, by the daemon's own card id. A card with no entry has never been read. */
  previews: Map<string, Preview>;
  /** The port the next card's dev server is given. Each card gets its own. */
  nextPort: number;
  /** The project's dev command, which is what the tab says pressing Start would run. */
  commandOf: (card: WireCard) => string;
  /** True while Marshal can find a browser to shoot with. False makes every screenshot `skipped`. */
  hasBrowser: boolean;
  /** Announces a change to every open stream, as the daemon's own bus does. */
  publish: Publish;
  /** Its clock, as the ISO string a preview's own times and a snapshot's `serverTime` came from. */
  now: () => string;
}

export interface FakePreviewOptions {
  /** The previews it starts with, one per card. None by default: every card reads as stopped. */
  previews?: readonly Preview[];
  /** The project's dev command. Empty for every project by default. */
  commandOf?: (card: WireCard) => string;
  /** Whether Marshal can find a browser. True by default, so a screenshot is taken. */
  hasBrowser?: boolean;
  /** Where a changed preview is announced. Discarded by default, for a test that reads the store. */
  publish?: Publish;
  /** Its clock, as an ISO string. The wall clock by default. */
  now?: () => string;
}

export function createPreviewStore(options: FakePreviewOptions = {}): PreviewStore {
  const previews = new Map<string, Preview>();
  for (const preview of options.previews ?? []) previews.set(preview.cardId, preview);
  return {
    previews,
    nextPort: FIRST_PORT,
    commandOf: options.commandOf ?? (() => ""),
    hasBrowser: options.hasBrowser ?? true,
    publish: options.publish ?? (() => undefined),
    now: options.now ?? (() => new Date().toISOString()),
  };
}

/** How many named segments come before a preview's own: `["v1", "cards", <id>, "preview"]`. */
const PREVIEW_PATH_SEGMENTS = 4;

/**
 * A card's preview for a test that reads it before any route was called, written the way the daemon
 * writes it: stopped, with the project's dev command already in it.
 */
function previewFor(store: PreviewStore, card: WireCard): Preview {
  const held = store.previews.get(card.id);
  if (held) return held;
  return {
    cardId: card.id,
    state: PreviewStateStopped,
    url: "",
    port: 0,
    command: store.commandOf(card),
    startedAt: null,
    error: "",
    shots: [],
  };
}

const snapshotOf = (store: PreviewStore, preview: Preview): PreviewSnapshot => ({
  preview,
  serverTime: store.now(),
});

/** Says a card's preview changed, on the card's own topic, as the daemon's own `publish` does. */
function announce(store: PreviewStore, preview: Preview): void {
  store.publish(`card:${preview.cardId}`, EventTypePreviewStateChanged, { preview });
}

/**
 * A card's preview as it is now. Reading starts nothing: a person looking at the tab must not start
 * a dev server on the machine by looking, which is why this is the only route that asks for nothing.
 */
export function answerPreviewRoute(
  store: PreviewStore,
  card: WireCard,
  segments: readonly string[],
  request: FakeRequest,
): Response | undefined {
  const rest = segments.slice(PREVIEW_PATH_SEGMENTS);
  if (request.method === "GET" && rest.length === 0) {
    return jsonAnswer(snapshotOf(store, previewFor(store, card)));
  }
  if (request.method === "POST" && rest.length === 1) {
    if (rest[0] === "start") return start(store, card);
    if (rest[0] === "stop") return stop(store, card);
    if (rest[0] === "shots") return screenshot(store, card, request);
  }
  // The image itself, named by the file the shot's own address carries: `before.png` or `after.png`.
  if (request.method === "GET" && rest.length === 2 && rest[0] === "shots") {
    return shotImage(store, card, rest[1] ?? "");
  }
  return undefined;
}

/**
 * Runs the card's dev command on a port of its own. A project with no dev command is refused in a
 * plain sentence rather than shown a spinner that never ends, which is the daemon's own rule.
 * Starting a preview that is already running changes nothing and answers it.
 */
function start(store: PreviewStore, card: WireCard): Response {
  const command = store.commandOf(card);
  if (!command) {
    return errorAnswer(STATUS.refused, "refused", NO_COMMAND);
  }
  const held = store.previews.get(card.id);
  if (held?.state === PreviewStateRunning || held?.state === PreviewStateStarting) {
    return jsonAnswer(snapshotOf(store, held));
  }
  const port = store.nextPort;
  store.nextPort += 1;
  const running: Preview = {
    cardId: card.id,
    state: PreviewStateRunning,
    url: `http://127.0.0.1:${String(port)}`,
    port,
    command,
    startedAt: store.now(),
    error: "",
    // A card that already has shots keeps them: the daemon's own live keeps its pair across a stop.
    shots: held?.shots ?? [],
  };
  store.previews.set(card.id, running);
  announce(store, running);
  return jsonAnswer(snapshotOf(store, running));
}

/**
 * Stops the card's dev server and answers the preview as it is now: `stopped`. Stopping a preview
 * that is not running is not an error, and the port and the shots it already had are kept, exactly
 * as the daemon's own `markStopped` keeps them.
 */
function stop(store: PreviewStore, card: WireCard): Response {
  const held = previewFor(store, card);
  const stopped: Preview = {
    ...held,
    state: PreviewStateStopped,
    url: "",
    startedAt: null,
    error: "",
  };
  store.previews.set(card.id, stopped);
  announce(store, stopped);
  return jsonAnswer(snapshotOf(store, stopped));
}

const isShotKind = (kind: unknown): kind is PreviewShotKind =>
  PreviewShotKindValues.some((known) => known === kind);

/**
 * Takes one half of a running preview's pair. A kind Marshal does not know is an invalid argument;
 * a preview that is not running is refused, because there is nothing to capture; and a machine with
 * no browser answers `skipped` with the sentence that says why, since a check that never ran must
 * never read as one that passed.
 */
function screenshot(store: PreviewStore, card: WireCard, request: FakeRequest): Response {
  const body = JSON.parse(request.body ?? "{}") as PreviewShotRequest;
  if (!isShotKind(body.kind)) {
    return errorAnswer(STATUS.badRequest, "invalid_argument", NO_SUCH_KIND);
  }
  const held = previewFor(store, card);
  if (held.state !== PreviewStateRunning) {
    return errorAnswer(STATUS.refused, "refused", NOT_RUNNING);
  }
  if (!store.hasBrowser) {
    return jsonAnswer({
      preview: held,
      outcome: PreviewShotOutcomeSkipped,
      notice: NO_BROWSER,
      serverTime: store.now(),
    } satisfies PreviewShotResult);
  }
  const takenAt = store.now();
  const shot: PreviewShot = {
    kind: body.kind,
    url: `/v1/cards/${card.id}/preview/shots/${body.kind}.png?v=${String(Date.parse(takenAt))}`,
    width: 1280,
    height: 720,
    takenAt,
  };
  const withShot: Preview = { ...held, shots: putShot(held.shots, shot) };
  store.previews.set(card.id, withShot);
  announce(store, withShot);
  return jsonAnswer({
    preview: withShot,
    outcome: PreviewShotOutcomeTaken,
    notice: TAKEN,
    serverTime: store.now(),
  } satisfies PreviewShotResult);
}

/** Records a shot, replacing an earlier one of the same kind, and keeps the pair before, then after. */
function putShot(shots: readonly PreviewShot[], shot: PreviewShot): PreviewShot[] {
  const kept = shots.filter((existing) => existing.kind !== shot.kind);
  kept.push(shot);
  return kept.sort(
    (a, b) => Number(b.kind === PreviewShotKindBefore) - Number(a.kind === PreviewShotKindBefore),
  );
}

/**
 * The image of one shot, which is only there while the preview holds that kind: a card with no such
 * shot is not found, the way the daemon's own `ShotFile` answers when the file is gone.
 */
function shotImage(store: PreviewStore, card: WireCard, name: string): Response {
  const kind = name.endsWith(".png") ? name.slice(0, -".png".length) : "";
  const held = store.previews.get(card.id);
  if (!isShotKind(kind) || !held?.shots.some((shot) => shot.kind === kind)) {
    return errorAnswer(
      STATUS.notFound,
      "not_found",
      "Marshal cannot find that screenshot. It may have been removed.",
    );
  }
  return new Response(SHOT_PNG, {
    status: 200,
    headers: { "Content-Type": "image/png", "X-Content-Type-Options": "nosniff" },
  });
}
