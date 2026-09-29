/**
 * The card panel routes of the fake daemon (docs/backend-checklist.md B10.2, B10.5, B10.6, sections
 * S12, S15, S16): a card's acceptance checks, checklists, comments, and members. Like the daemon,
 * every call answers the whole list, and every change is announced on the card's own topic with the
 * event the daemon publishes (`checklist.updated`, `comment.created`, `card.members_changed`).
 *
 * The rules that matter to a screen are kept: a people-only list refuses the agent, a comment that
 * mentions @agent or ends in a question mark is marked read once the agent takes it, a person deletes
 * only their own comment, and a run of the checks records a run reference.
 */
import type {
  AddCardCheckRequest,
  AddChecklistItemRequest,
  Attachment,
  CardCheck,
  CardCheckList,
  CardMembers,
  Checklist,
  ChecklistList,
  Comment,
  CommentList,
  CreateChecklistRequest,
  PostCommentRequest,
  TickChecklistItemRequest,
  UpdateChecklistRequest,
} from "@marshal/protocol";
import { emptyAnswer, errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";

type Publish = (topic: string, type: string, data: unknown) => void;

const ME = "01M3USER00000000000000000A";
const MAX_CHECKLISTS = 20;
const ID_LENGTH = 26;
const BAD_REQUEST = 400;
const FORBIDDEN = 403;
const NOT_FOUND = 404;
const NOT_ALLOWED = 405;

interface PanelCard {
  checks: CardCheck[];
  lists: Checklist[];
  comments: Comment[];
  members: string[];
  /** The bytes of each kept file, by attachment id. */
  files: Map<string, { name: string; mime: string; data: string }>;
}

export interface CardPanelStore {
  cards: Map<string, PanelCard>;
  /** Who a person's own change is by. */
  meId: () => string;
  publish: Publish;
  now: () => string;
  nextId: number;
  /** The users a member may be, by id. A card's member who is not one of them is not found. */
  users: Set<string>;
  /** The checks a card starts with. */
  defaults: () => CardCheck[];
  /** Ids of the checks that fail when the checks run, so a test can draw a failure. */
  failing: Set<string>;
}

export interface FakeCardPanelOptions {
  meId?: () => string;
  publish?: Publish;
  now?: () => string;
  users?: readonly string[];
}

export function createCardPanelStore(options: FakeCardPanelOptions = {}): CardPanelStore {
  const store: CardPanelStore = {
    cards: new Map(),
    meId: options.meId ?? (() => ME),
    publish: options.publish ?? (() => undefined),
    now: options.now ?? (() => new Date().toISOString()),
    nextId: 1,
    users: new Set(options.users ?? [ME]),
    defaults: () => [],
    failing: new Set(),
  };
  store.defaults = () => [
    check(store, "Tests pass", "command", "go test ./..."),
    check(store, "Lint clean", "command", "go vet ./..."),
    check(store, "Reviewer approval", "review", ""),
  ];
  return store;
}

const id = (store: CardPanelStore, prefix: string): string =>
  `${prefix}${String(store.nextId++).padStart(ID_LENGTH - prefix.length, "0")}`.toUpperCase();

function check(store: CardPanelStore, name: string, kind: string, command: string): CardCheck {
  return { id: id(store, "01M3CHECK"), name, kind, command, status: "pending", runRef: "" };
}

function panelOf(store: CardPanelStore, cardId: string): PanelCard {
  let panel = store.cards.get(cardId);
  if (!panel) {
    panel = { checks: store.defaults(), lists: [], comments: [], members: [], files: new Map() };
    store.cards.set(cardId, panel);
  }
  return panel;
}

const notFound = (what: string): Response =>
  errorAnswer(
    NOT_FOUND,
    "not_found",
    `Marshal cannot find that ${what}. It may have been removed.`,
  );
const invalid = (message: string): Response =>
  errorAnswer(BAD_REQUEST, "invalid_argument", message);
const stamp = (store: CardPanelStore): string => store.now();

function bodyOf<T>(request: FakeRequest): T {
  try {
    return JSON.parse(request.body ?? "{}") as T;
  } catch {
    return {} as T;
  }
}

const checksAnswer = (store: CardPanelStore, panel: PanelCard): Response =>
  jsonAnswer({ checks: panel.checks, serverTime: stamp(store) } satisfies CardCheckList);
const listsAnswer = (store: CardPanelStore, panel: PanelCard): Response =>
  jsonAnswer({ checklists: panel.lists, serverTime: stamp(store) } satisfies ChecklistList);
const commentsAnswer = (store: CardPanelStore, panel: PanelCard): Response =>
  jsonAnswer({ comments: panel.comments, serverTime: stamp(store) } satisfies CommentList);
const membersAnswer = (store: CardPanelStore, panel: PanelCard): Response =>
  jsonAnswer({ userIds: panel.members, serverTime: stamp(store) } satisfies CardMembers);

const topic = (cardId: string): string => `card:${cardId}`;
const announce = (store: CardPanelStore, cardId: string, type: string): void =>
  store.publish(topic(cardId), type, { cardId });

function checksRoute(store: CardPanelStore, cardId: string, rest: string[], request: FakeRequest) {
  const panel = panelOf(store, cardId);
  const [first] = rest;
  if (request.method === "GET" && !first) return checksAnswer(store, panel);
  if (request.method === "POST" && !first) {
    const req = bodyOf<AddCardCheckRequest>(request);
    if (!req.name?.trim() || !req.command?.trim())
      return invalid("A check needs a name and a command.");
    panel.checks.push(check(store, req.name.trim(), "command", req.command.trim()));
    return checksAnswer(store, panel);
  }
  if (request.method === "POST" && first === "run") {
    const ref = `run-${store.nextId++}`;
    for (const one of panel.checks) {
      if (one.kind !== "command") continue;
      one.status = store.failing.has(one.name) ? "failed" : "passed";
      one.runRef = ref;
    }
    unticked(store, cardId, panel);
    return checksAnswer(store, panel);
  }
  if (request.method === "DELETE" && first) {
    panel.checks = panel.checks.filter((one) => one.id !== first);
    return checksAnswer(store, panel);
  }
  return undefined;
}

/** Nothing here keeps proofs, so a run only announces that the lists may have changed. */
function unticked(store: CardPanelStore, cardId: string, _panel: PanelCard): void {
  announce(store, cardId, "checklist.updated");
}

function itemRoute(
  store: CardPanelStore,
  cardId: string,
  list: Checklist,
  rest: string[],
  request: FakeRequest,
  panel: PanelCard,
): Response | undefined {
  const [, itemId] = rest;
  if (rest[0] !== "items") return undefined;
  if (request.method === "POST" && !itemId) {
    const req = bodyOf<AddChecklistItemRequest>(request);
    if (!req.text?.trim()) return invalid("A line needs some text, at most 500 characters.");
    list.items.push({
      id: id(store, "01M3ITEM"),
      text: req.text.trim(),
      done: false,
      doneByKind: "",
      doneById: "",
      doneAt: null,
    });
    announce(store, cardId, "checklist.updated");
    return listsAnswer(store, panel);
  }
  const item = list.items.find((one) => one.id === itemId);
  if (!item) return notFound("checklist item");
  if (request.method === "PUT") {
    const req = bodyOf<TickChecklistItemRequest>(request);
    item.done = !!req.done;
    item.doneByKind = item.done ? "person" : "";
    item.doneById = item.done ? store.meId() : "";
    item.doneAt = stamp(store);
  } else if (request.method === "DELETE") {
    list.items = list.items.filter((one) => one !== item);
  } else {
    return undefined;
  }
  announce(store, cardId, "checklist.updated");
  return listsAnswer(store, panel);
}

function checklistsRoute(
  store: CardPanelStore,
  cardId: string,
  rest: string[],
  request: FakeRequest,
) {
  const panel = panelOf(store, cardId);
  const [listId, ...more] = rest;
  if (!listId) {
    if (request.method === "GET") return listsAnswer(store, panel);
    if (request.method !== "POST") return undefined;
    if (panel.lists.length >= MAX_CHECKLISTS)
      return invalid("A card can have at most 20 checklists.");
    const req = bodyOf<CreateChecklistRequest>(request);
    panel.lists.push({
      id: id(store, "01M3LIST"),
      name: req.name?.trim() || "Checklist",
      required: false,
      peopleOnly: false,
      hideChecked: false,
      items: [],
    });
    announce(store, cardId, "checklist.updated");
    return listsAnswer(store, panel);
  }
  const list = panel.lists.find((one) => one.id === listId);
  if (!list) return notFound("checklist");
  if (more.length) return itemRoute(store, cardId, list, more, request, panel);
  if (request.method === "PATCH") {
    const req = bodyOf<UpdateChecklistRequest>(request);
    if (req.name !== undefined) {
      if (!req.name.trim()) return invalid("A checklist needs a name of at most 100 characters.");
      list.name = req.name.trim();
    }
    if (req.required !== undefined) list.required = req.required;
    if (req.peopleOnly !== undefined) list.peopleOnly = req.peopleOnly;
    if (req.hideChecked !== undefined) list.hideChecked = req.hideChecked;
  } else if (request.method === "DELETE") {
    panel.lists = panel.lists.filter((one) => one !== list);
  } else {
    return undefined;
  }
  announce(store, cardId, "checklist.updated");
  return listsAnswer(store, panel);
}

const AGENT_MENTION = /(^|[^\w@])@agent\b/i;
const asksAgent = (body: string): boolean => AGENT_MENTION.test(body) || body.trim().endsWith("?");
const LINK = /https?:\/\/[^\s)]+/g;

function post(store: CardPanelStore, cardId: string, panel: PanelCard, request: FakeRequest) {
  const req = bodyOf<PostCommentRequest>(request);
  const body = (req.body ?? "").trim();
  const attachments: Attachment[] = [];
  const seen = new Set<string>();
  const addLink = (url: string): void => {
    if (seen.has(url)) return;
    seen.add(url);
    attachments.push({
      id: id(store, "01M3ATTA"),
      kind: "link",
      name: url.replace(/^https?:\/\//, ""),
      sizeBytes: 0,
      url,
    });
  };
  for (const one of req.attachments ?? []) {
    if (one.kind === "link") {
      addLink(one.url);
      continue;
    }
    const attachment: Attachment = {
      id: id(store, "01M3ATTA"),
      kind: one.kind,
      name: one.name,
      sizeBytes: atob(one.data).length,
      url: "",
    };
    panel.files.set(attachment.id, { name: one.name, mime: one.mimeType, data: one.data });
    attachments.push(attachment);
  }
  for (const url of body.match(LINK) ?? []) addLink(url.replace(/[.,;:!?]+$/, ""));
  if (!body && !attachments.length) return invalid("Write something or attach something first.");
  const comment: Comment = {
    id: id(store, "01M3COMM"),
    authorKind: "person",
    authorId: store.meId(),
    body,
    attachments,
    agentReadAt: asksAgent(body) ? stamp(store) : null,
    createdAt: stamp(store),
  };
  panel.comments.push(comment);
  announce(store, cardId, "comment.created");
  return commentsAnswer(store, panel);
}

function commentsRoute(
  store: CardPanelStore,
  cardId: string,
  rest: string[],
  request: FakeRequest,
) {
  const panel = panelOf(store, cardId);
  const [commentId] = rest;
  if (!commentId) {
    if (request.method === "GET") return commentsAnswer(store, panel);
    return request.method === "POST" ? post(store, cardId, panel, request) : undefined;
  }
  if (request.method !== "DELETE") return undefined;
  const comment = panel.comments.find((one) => one.id === commentId);
  if (!comment) return notFound("comment");
  if (comment.authorKind !== "person" || comment.authorId !== store.meId()) {
    return errorAnswer(FORBIDDEN, "forbidden", "You can only delete your own comments.");
  }
  panel.comments = panel.comments.filter((one) => one !== comment);
  announce(store, cardId, "comment.created");
  return commentsAnswer(store, panel);
}

function membersRoute(store: CardPanelStore, cardId: string, rest: string[], request: FakeRequest) {
  const panel = panelOf(store, cardId);
  const [userId] = rest;
  if (!userId) return request.method === "GET" ? membersAnswer(store, panel) : undefined;
  if (request.method === "PUT") {
    if (!store.users.has(userId)) return notFound("person");
    if (!panel.members.includes(userId)) panel.members.push(userId);
  } else if (request.method === "DELETE") {
    panel.members = panel.members.filter((one) => one !== userId);
  } else {
    return undefined;
  }
  announce(store, cardId, "card.members_changed");
  return membersAnswer(store, panel);
}

function attachmentRoute(store: CardPanelStore, cardId: string, rest: string[]) {
  const file = panelOf(store, cardId).files.get(rest[0] ?? "");
  if (!file) return notFound("attachment");
  const bytes = Uint8Array.from(atob(file.data), (char) => char.charCodeAt(0));
  return new Response(bytes, {
    status: 200,
    headers: { "Content-Type": file.mime || "application/octet-stream" },
  });
}

const CARD_ROUTE =
  /^\/v1\/cards\/([^/]+)\/(checks|checklists|comments|members|attachments)(?:\/(.*))?$/;

/** Answers one card panel route, or undefined when the request is not one. */
export function answerCardPanelRoute(
  store: CardPanelStore,
  request: FakeRequest,
): Response | undefined {
  const match = CARD_ROUTE.exec(new URL(request.url, "http://fake-daemon").pathname);
  if (!match) return undefined;
  const [, cardId = "", kind, tail] = match;
  const rest = tail ? tail.split("/") : [];
  if (kind === "checks") return checksRoute(store, cardId, rest, request);
  if (kind === "checklists") return checklistsRoute(store, cardId, rest, request);
  if (kind === "comments") return commentsRoute(store, cardId, rest, request);
  if (kind === "members") return membersRoute(store, cardId, rest, request);
  return request.method === "GET" ? attachmentRoute(store, cardId, rest) : emptyAnswer(NOT_ALLOWED);
}
