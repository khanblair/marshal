/**
 * The routing of the fake daemon's card routes, and the board, the cards, the diff, and the
 * project's own routes, so the unit and component tests that follow the cutover can drive the real
 * API client and the real mirror without a network. Each one answers the way the daemon's own
 * handler does, including the event it publishes after a change: a test that moves a card sees the
 * `card.moved` event arrive on the project's topic.
 *
 * The store these routes change, the wire fixtures, and the pieces they share are in
 * fake-card-shared; the actions of one card are in fake-card-actions, the project's labels in
 * fake-card-labels, and Home's numbers in fake-home.
 */
import type {
  CardDiff,
  CreateCardRequest,
  FileHunks,
  UpdateCardRequest,
  Card as WireCard,
} from "@marshal/protocol";
import { emptyAnswer, errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { cardAction, cardRead } from "./fake-card-actions";
import { createLabel, labelRoute, labelsOf } from "./fake-card-labels";
import {
  BOARD_COLUMNS,
  type CardStore,
  cardOf,
  freshCardId,
  notFound,
  publishCard,
  type Route,
  STATUS,
  topic,
  wireCard,
} from "./fake-card-shared";
import { homeActivity, homeDashboard } from "./fake-home";

const noProject = () =>
  errorAnswer(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that project. It may have been removed.",
  );

const notFoundFile = (): Response =>
  errorAnswer(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that file. It may have been removed.",
  );

/** A card's changed files with their counts, and no hunks (section S11). */
function cardDiff(store: CardStore, card: WireCard): Response {
  const found = store.diffs[card.id];
  const files = found?.files ?? [];
  const answer: CardDiff = {
    cardId: card.id,
    base: found?.base ?? "main",
    branch: found?.branch ?? "",
    files,
    fileCount: files.length,
    additions: files.reduce((sum, file) => sum + file.additions, 0),
    deletions: files.reduce((sum, file) => sum + file.deletions, 0),
    truncated: false,
    serverTime: store.now(),
  };
  return jsonAnswer(answer);
}

/** One changed file's hunks, loaded when the screen opens it (section S11). */
function fileHunks(store: CardStore, card: WireCard, path: string): Response {
  const file = store.diffs[card.id]?.files.find((one) => one.path === path);
  if (!file) return notFoundFile();
  const answer: FileHunks = {
    path,
    status: file.status,
    hunks: store.diffs[card.id]?.hunks?.[path] ?? [],
    truncated: false,
    serverTime: store.now(),
  };
  return jsonAnswer(answer);
}

function createCard(store: CardStore, projectId: string, request: FakeRequest): Response {
  const body = JSON.parse(request.body ?? "{}") as CreateCardRequest;
  if (!body.title?.trim()) {
    return errorAnswer(STATUS.badRequest, "invalid_argument", "Give the card a name.");
  }
  const numbers = store.cards
    .filter((card) => card.projectId === projectId)
    .map((card) => card.number);
  const number = Math.max(0, ...numbers) + 1;
  // The real route writes the card in the backlog and starts its session straight after when
  // StartState is planning or working, so the answer carries the column it was asked for. A state
  // that is not one of the three addable columns is refused there; here the board's own columns are
  // accepted and anything else falls back to the backlog.
  const start =
    body.startState && BOARD_COLUMNS.includes(body.startState) ? body.startState : "backlog";
  const card = wireCard({
    id: freshCardId(),
    projectId,
    number,
    title: body.title.trim(),
    body: body.body ?? "",
    state: start,
    agent: body.agent || "claude",
    model: body.model ?? "",
    thinking: body.thinking || null,
    permissionMode: body.permissionMode || "auto-edits",
    role: body.role ?? "",
    package: body.package ?? "",
    // The daemon writes a "doing now" line only when a card is created with one (a fork) or when a
    // person edits it, so a card started by its create carries none.
    doingNow: "",
    createdAt: store.now(),
    updatedAt: store.now(),
  });
  store.cards.push(card);
  publishCard(store, card, "card.created");
  return jsonAnswer(card, STATUS.created);
}

function updateCard(store: CardStore, card: WireCard, request: FakeRequest): Response {
  const body = JSON.parse(request.body ?? "{}") as UpdateCardRequest;
  if (body.title !== undefined && !body.title?.trim()) {
    return errorAnswer(STATUS.badRequest, "invalid_argument", "Card names can't be empty.");
  }
  // A field that is not in the body is not touched, which is the whole point of the route.
  const fields: (keyof UpdateCardRequest)[] = [
    "title",
    "body",
    "agent",
    "model",
    "thinking",
    "permissionMode",
    "role",
    "package",
  ];
  for (const field of fields) {
    const value = body[field];
    if (value !== undefined) Object.assign(card, { [field]: value });
  }
  for (const date of ["plannedStart", "plannedEnd", "due", "actualStart", "actualEnd"] as const) {
    const change = body[date];
    if (change === undefined) continue;
    Object.assign(card, { [date]: change.clear ? null : (change.at ?? null) });
  }
  if (body.labels != null) card.labels = labelsOf(store, body.labels);
  card.updatedAt = store.now();
  publishCard(store, card, "card.updated");
  return jsonAnswer(card);
}

function removeCard(store: CardStore, card: WireCard): Response {
  store.cards.splice(store.cards.indexOf(card), 1);
  store.publish(topic(card.projectId), "card.deleted", {
    cardId: card.id,
    key: card.key,
    projectId: card.projectId,
  });
  return emptyAnswer();
}

/** The board of one project, in the daemon's own order: by number. */
function board(store: CardStore, projectId: string): Response {
  const cards = store.cards
    .filter((card) => card.projectId === projectId)
    .sort((a, b) => a.number - b.number);
  return jsonAnswer({
    projectId,
    columns: BOARD_COLUMNS,
    cards,
    serverTime: store.now(),
  });
}

/** One request under `/v1/cards`, `/v1/labels`, `/v1/home`, or the board and label routes of a
 * project. Returns undefined for a path that belongs to none of them, so the caller can answer 404
 * the way the real daemon's router does.
 */
export function answerCardRoute(
  store: CardStore,
  request: FakeRequest,
  projectExists: (id: string) => boolean,
  projectNames: (id: string) => string,
): Response | undefined {
  const [rawPath, query] = request.url.replace(/^https?:\/\/[^/]+/, "").split("?");
  const path = rawPath ?? "";
  // `["v1", <kind>, <id>, <action>, <extra>]`: named, so no path index is a bare number. The
  // segments are escaped in the address, and the app names a card by its key (`api#41`), which has
  // a character the client escapes, so each one is read back the way the real router's would be.
  const segments = path
    .split("/")
    .filter(Boolean)
    .map((segment) => decodeURIComponent(segment));
  const [, kind, first, second, third] = segments;
  const route: Route = { store, request, method: request.method };

  // Home's dashboard numbers and its activity stream are their own group (fake-home).
  if (kind === "home") {
    const params = new URLSearchParams(query);
    if (first === "dashboard" && !second) return homeDashboard(store, projectNames, params);
    if (first === "activity" && !second) return homeActivity(store, params);
  }
  // A project route has at most one segment after the id (its board, its cards, its labels).
  if (kind === "projects" && first && !third) {
    return projectRoute(route, first, second, projectExists(first));
  }
  if (kind === "labels" && first && !second) return labelRoute(route, first);
  if (kind === "cards" && first && second === "diff") {
    return diffRoute(store, first, segments);
  }
  if (kind === "cards" && first) return cardRoute(route, first, second, third);
  return undefined;
}

/** How many named segments come before a diff file path: `["v1", "cards", <id>, "diff"]`. */
const DIFF_PATH_SEGMENTS = 4;

/** The diff routes take the rest of the address as one file path (a real path wildcard, not a
 * named segment), so they read the segments directly instead of going through cardRoute. */
function diffRoute(store: CardStore, cardId: string, segments: readonly string[]): Response {
  const card = cardOf(store, cardId);
  if (!card) return notFound();
  const filePath = segments.slice(DIFF_PATH_SEGMENTS).join("/");
  return filePath ? fileHunks(store, card, filePath) : cardDiff(store, card);
}

/** A route under one project: its board, its cards, or its labels. */
function projectRoute(
  { store, request, method }: Route,
  projectId: string,
  action: string | undefined,
  exists: boolean,
): Response | undefined {
  if (action === "board") return exists ? board(store, projectId) : noProject();
  if (action === "cards" && method === "POST") {
    return exists ? createCard(store, projectId, request) : noProject();
  }
  if (action !== "labels") return undefined;
  if (!exists) return noProject();
  if (method === "GET") {
    return jsonAnswer({
      projectId,
      labels: store.labels.filter((label) => label.projectId === projectId),
      serverTime: store.now(),
    });
  }
  if (method === "POST") return createLabel(store, projectId, request);
  return undefined;
}

/** A route under one card: read it, change it, move it, fork it, or remove it. */
function cardRoute(
  { store, request, method }: Route,
  id: string,
  action: string | undefined,
  extra: string | undefined,
): Response | undefined {
  const card = cardOf(store, id);
  if (!card) return notFound();
  if (method === "GET") return cardRead(store, card, request, action, extra);
  if (extra) return undefined;
  if (!action) {
    if (method === "PATCH") return updateCard(store, card, request);
    if (method === "DELETE") return removeCard(store, card);
    return undefined;
  }
  if (method !== "POST") return undefined;
  return cardAction(store, card, request, action);
}

export type { CardStore, DaemonBoard, FakeCardDiff, HistoryRow } from "./fake-card-shared";
/** The fixtures a test makes a card with, and the shapes these routes and their neighbours take. */
export { boardOf, cardId, historyRow, wireCard } from "./fake-card-shared";
