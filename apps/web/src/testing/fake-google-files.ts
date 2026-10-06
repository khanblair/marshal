/**
 * The Google file routes of the fake daemon (S29i to S29l): consent URL, files, creates, read by link.
 * A service that is not connected is refused with the daemon's own sentence. Nothing reaches Google:
 * a create keeps a made-up file, and a read answers a small made-up document.
 */
import type { GoogleFile, GoogleFileKind, GoogleFiles, GoogleLinkContent } from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { GOOGLE_FILE_SERVICES, type IntegrationStore } from "./fake-integrations";

const STATUS = { created: 201, badRequest: 400, unprocessable: 422 };
const MAX_LISTED = 20;
const MAX_NAME_CHARS = 200;
const AUTHORIZE_PATH = /^\/v1\/integrations\/([^/]+)\/authorize$/;
const FILES_PATH = "/v1/google/files";
const READ_PATH = "/v1/google/read";
const LINK_PATH = /^\/(document|spreadsheets|presentation)(?:\/u\/\d+)?\/d\/([^/]+)/;
const LINK_KINDS: Readonly<Record<string, GoogleFileKind>> = {
  document: "doc",
  spreadsheets: "sheet",
  presentation: "slides",
};
const NOT_A_LINK = "Paste the address of a Google Doc, Sheet or Slides presentation.";

/** What each create route makes: the kind of file, the connection whose access it needs, and its address. */
const CREATES: Readonly<
  Record<string, { kind: GoogleFileKind; service: string; address: string }>
> = {
  "/v1/google/docs": {
    kind: "doc",
    service: "gdocs",
    address: "https://docs.google.com/document/d/{id}/edit",
  },
  "/v1/google/sheets": {
    kind: "sheet",
    service: "gsheets",
    address: "https://docs.google.com/spreadsheets/d/{id}/edit",
  },
  "/v1/google/slides": {
    kind: "slides",
    service: "gslides",
    address: "https://docs.google.com/presentation/d/{id}/edit",
  },
  "/v1/google/drive/files": {
    kind: "file",
    service: "gdrive",
    address: "https://drive.google.com/file/d/{id}/view",
  },
};

/** The connection a kind of file is listed and read with. A plain file, and no kind, use Drive's. */
const SERVICE_OF: Readonly<Record<GoogleFileKind, string>> = {
  doc: "gdocs",
  sheet: "gsheets",
  slides: "gslides",
  file: "gdrive",
};

/** The files the fake daemon holds, newest first. */
export interface GoogleFilesStore {
  files: GoogleFile[];
  /** The number the next file's id is made from. */
  seq: number;
  /** Its clock, as the ISO string a file's time is written from. */
  now: () => string;
}

export interface FakeGoogleFilesOptions {
  /** The files it starts with, newest first. None by default. */
  files?: readonly GoogleFile[];
  /** Its clock, as an ISO string. The wall clock by default. */
  now?: () => string;
}

export function createGoogleFilesStore(options: FakeGoogleFilesOptions = {}): GoogleFilesStore {
  return {
    files: structuredClone([...(options.files ?? [])]),
    seq: 0,
    now: options.now ?? (() => new Date().toISOString()),
  };
}

const refuse = (message: string): Response => errorAnswer(STATUS.unprocessable, "refused", message);

const invalid = (message: string): Response =>
  errorAnswer(STATUS.badRequest, "invalid_argument", message);

function bodyOf(request: FakeRequest): Record<string, unknown> {
  try {
    return request.body ? (JSON.parse(request.body) as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

/** The sentence a service that cannot be used gets, or null when its access is there. */
function unusable(connections: IntegrationStore, service: string): string | null {
  const name = GOOGLE_FILE_SERVICES[service]?.name ?? service;
  const row = connections.rows.find((entry) => entry.id === service);
  if (row?.st === "connected") return null;
  if (row?.st === "error") {
    return `Google no longer accepts Marshal's access to ${name}. Reconnect it in Settings.`;
  }
  return `${name} is not connected yet. Connect it in Settings, under Integrations.`;
}

const isKind = (value: string): value is GoogleFileKind => Object.hasOwn(SERVICE_OF, value);

function listFiles(store: GoogleFilesStore, connections: IntegrationStore, kind: string): Response {
  if (kind && !isKind(kind)) return invalid("Choose doc, sheet, slides or file.");
  const refusal = unusable(connections, kind && isKind(kind) ? SERVICE_OF[kind] : "gdrive");
  if (refusal) return refuse(refusal);
  const files = store.files.filter((file) => !kind || file.kind === kind).slice(0, MAX_LISTED);
  const answer: GoogleFiles = { files, folder: connections.driveFolder };
  return jsonAnswer(answer);
}

function createFile(
  store: GoogleFilesStore,
  connections: IntegrationStore,
  path: string,
  request: FakeRequest,
): Response {
  const spec = CREATES[path];
  if (!spec) return invalid("Marshal does not make that.");
  const refusal = unusable(connections, spec.service);
  if (refusal) return refuse(refusal);
  const body = bodyOf(request);
  const name = String(spec.kind === "file" ? (body.name ?? "") : (body.title ?? "")).trim();
  if (!name || name.length > MAX_NAME_CHARS) {
    return invalid("The title must be 1 to 200 characters.");
  }
  store.seq += 1;
  const id = `${spec.kind}-${store.seq}`;
  const file: GoogleFile = {
    id,
    name,
    kind: spec.kind,
    url: spec.address.replace("{id}", id),
    modifiedAt: store.now(),
  };
  store.files.unshift(file);
  return jsonAnswer(file, STATUS.created);
}

/** What a link names: the kind of file and its id, or null when it is not a Google address of one. */
function parseLink(url: unknown): { kind: GoogleFileKind; id: string } | null {
  let address: URL;
  try {
    address = new URL(String(url));
  } catch {
    return null;
  }
  if (address.hostname !== "docs.google.com") return null;
  const [, product = "", id = ""] = LINK_PATH.exec(address.pathname.replace(/^\/u\/\d+/, "")) ?? [];
  const kind = LINK_KINDS[product];
  return kind && id ? { kind, id } : null;
}

function readLink(connections: IntegrationStore, request: FakeRequest): Response {
  const link = parseLink(bodyOf(request).url);
  if (!link) return invalid(NOT_A_LINK);
  const refusal = unusable(connections, SERVICE_OF[link.kind]);
  if (refusal) return refuse(refusal);
  const content: GoogleLinkContent = {
    kind: link.kind,
    id: link.id,
    title: "Launch plan",
    url: String(bodyOf(request).url),
    markdown: "# Launch plan\n\n- Write the notes\n- Send them out",
    truncated: false,
  };
  return jsonAnswer(content);
}

/**
 * Answers one Google file route, or null when the request is not one. The connections are what
 * decide whether a service may be used, so a test connects one by changing its row.
 */
export function answerGoogleFilesRoute(
  store: GoogleFilesStore,
  connections: IntegrationStore,
  request: FakeRequest,
): Response | null {
  const url = new URL(request.url, "http://fake-daemon");
  const path = url.pathname;
  const authorize = AUTHORIZE_PATH.exec(path);
  const id = decodeURIComponent(authorize?.[1] ?? "");
  if (authorize && request.method === "GET" && GOOGLE_FILE_SERVICES[id]) {
    return jsonAnswer({ url: `https://accounts.google.com/o/oauth2/auth?fake=${id}` });
  }
  if (path === FILES_PATH && request.method === "GET") {
    return listFiles(store, connections, url.searchParams.get("kind") ?? "");
  }
  if (path === READ_PATH && request.method === "POST") return readLink(connections, request);
  if (request.method === "POST" && CREATES[path]) {
    return createFile(store, connections, path, request);
  }
  return null;
}
