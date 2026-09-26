import {
  type CardHit,
  type ChatHit,
  MaxSearchQueryChars,
  type ProjectHit,
  SearchHitsPerKind,
  type SearchSnapshot,
} from "@marshal/protocol";
import { batch, createSignal, getOwner, onCleanup } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { ApiError } from "~/data/api-error";
import { isDaemon } from "~/data/sections";
import { openChat } from "~/mock/actions/chats";
import { go, openCard } from "~/mock/actions/navigation";
import type { Command } from "~/mock/actions/palette";
import { cardLabelIn } from "~/mock/card-key";
import { STATUS, tone } from "~/mock/constants";
import { type Ctx, sectionsOf } from "~/mock/context";
import { toast } from "~/mock/engine";
import { card } from "~/mock/selectors";
import { applyCard } from "./cards";
import { applyChat } from "./chats";

/**
 * Section S24a: what the command palette finds when something is typed (docs/backend-checklist.md
 * B2.11). `GET /v1/search?q=` answers the projects, cards, and chats that match, each kind best
 * match first and cut to `SearchHitsPerKind`. The palette's own commands (its actions and its
 * settings) are not searched here: they stay on the client, and so does the palette's search of
 * what the store holds, which is what it shows until the daemon has answered, and for good when the
 * daemon is away or the request fails. So the palette is never empty for want of an answer.
 */

/** How long typing must pause before a search is sent, so a burst of keys is one request. */
export const SEARCH_DEBOUNCE_MS = 150;

const PROJECT_GROUP = "Projects";
const CARD_GROUP = "Cards";
const CHAT_GROUP = "Chats";

const PROJECT_ICON = "folder-git-2";
const CHAT_ICON = "messages-square";

/** The one sentence for a lookup that failed in a way the daemon gave no sentence for. */
const LOOKUP_FAILED = "Marshal could not open that result. Try again.";

/**
 * A query as the daemon reads it: trimmed, each run of white space made one space, and not lower
 * cased. The daemon answers with the query it searched, so this is the form an answer is compared
 * against.
 */
export const cleanQuery = (text: string): string => text.trim().replace(/\s+/g, " ");

/** The rows an answer made, for the query it answered, in the palette's own row shape. */
export interface PaletteHits {
  /** The cleaned query these rows answer. The palette uses them only while it still is the query. */
  query: string;
  projects: Command[];
  cards: Command[];
  chats: Command[];
}

/** One search session: made when the palette opens, and stopped when it closes. */
export interface PaletteSearch {
  /** The daemon's answer to the newest query asked, or null while there is none to show. */
  hits: () => PaletteHits | null;
  /** Tells the search what the field holds now. It sends nothing for a blank query. */
  ask: (text: string) => void;
  /** Cancels what is waiting or in flight. Called for you when the owner is disposed. */
  stop: () => void;
}

type Hit =
  | { kind: "project"; hit: ProjectHit }
  | { kind: "card"; hit: CardHit }
  | { kind: "chat"; hit: ChatHit };

/** The daemon's own wording for a thing it cannot find (`protocol.NotFound`). */
const notFound = (what: string): string =>
  `Marshal cannot find that ${what}. It may have been removed.`;

const holdsProject = (ctx: Ctx, id: string): boolean => ctx.S.projects.some((p) => p.id === id);

/**
 * True when the project is in the store. A project the store does not have yet (the daemon knows
 * one that the store has not been told about) is read for by loading everything again: the
 * projects come first, and the boards and chats of a project that arrives come with it.
 */
async function ensureProject(ctx: Ctx, id: string): Promise<boolean> {
  if (holdsProject(ctx, id)) return true;
  await ctx.sync?.reload();
  return holdsProject(ctx, id);
}

/** Opens a card, reading it first when the store does not hold it yet. */
async function showCard(ctx: Ctx, hit: CardHit): Promise<void> {
  const api = ctx.env.data?.api;
  if (!card(ctx, hit.key) && api) applyCard(ctx, await api.getCard(hit.cardId));
  if (!card(ctx, hit.key)) {
    toast(ctx, notFound("card"));
    return;
  }
  batch(() => openCard(ctx, hit.key));
}

/** Opens a chat in its project's chat view, reading the project's chats first when it is not held yet. */
async function showChat(ctx: Ctx, hit: ChatHit): Promise<void> {
  const known = (): boolean => (ctx.S.chats[hit.projectId] ?? []).some((c) => c.id === hit.chatId);
  const api = ctx.env.data?.api;
  if (!known() && api) {
    for (const chat of (await api.listChats(hit.projectId)).chats) applyChat(ctx, chat);
  }
  if (!known()) {
    toast(ctx, notFound("chat"));
    return;
  }
  // Both at once, so the chat view finds its chat chosen and does not choose the first one itself.
  batch(() => {
    go(ctx, "project", hit.projectId, "chat");
    openChat(ctx, hit.projectId, hit.chatId);
  });
}

/**
 * Opens what a row names, as the mock's own rows do: a project opens on its last view, a card
 * opens in its panel, and a chat opens in its project's chat view. A refusal shows the daemon's own
 * sentence and changes nothing.
 */
async function openHit(ctx: Ctx, target: Hit): Promise<void> {
  try {
    if (!(await ensureProject(ctx, target.hit.projectId))) {
      toast(ctx, notFound("project"));
    } else if (target.kind === "project") {
      batch(() => go(ctx, "project", target.hit.projectId));
    } else if (target.kind === "card") {
      await showCard(ctx, target.hit);
    } else {
      await showChat(ctx, target.hit);
    }
  } catch (error) {
    toast(ctx, error instanceof ApiError ? error.message : LOOKUP_FAILED);
  }
}

const opening = (ctx: Ctx, target: Hit) => (): void => {
  void openHit(ctx, target);
};

function projectRow(ctx: Ctx, hit: ProjectHit): Command {
  return {
    group: PROJECT_GROUP,
    label: hit.name,
    icon: PROJECT_ICON,
    hint: hit.language,
    run: opening(ctx, { kind: "project", hit }),
  };
}

/** A card row reads like the mock's: the project's name, the number, and the title. */
function cardRow(ctx: Ctx, hit: CardHit): Command {
  const status = STATUS[hit.state];
  return {
    group: CARD_GROUP,
    label: `${cardLabelIn({ n: hit.number }, hit.projectName)} ${hit.title}`,
    icon: status.icon,
    iconColor: tone(status.tone, "solid"),
    card: hit.key,
    run: opening(ctx, { kind: "card", hit }),
  };
}

/** A chat row names its project where a project row shows its language, in the palette's hint. */
function chatRow(ctx: Ctx, hit: ChatHit): Command {
  return {
    group: CHAT_GROUP,
    label: hit.title,
    icon: CHAT_ICON,
    hint: hit.projectName,
    run: opening(ctx, { kind: "chat", hit }),
  };
}

/**
 * The palette's rows for one answer. Each kind is cut to `SearchHitsPerKind` here too, so a daemon
 * that sent more could not fill the palette. Chat rows are left out while the chats are the mock's
 * (section S17): the daemon's chat ids are not the mock's, so a row would open an empty pane.
 */
export function hitsOf(ctx: Ctx, answer: SearchSnapshot): PaletteHits {
  const chats = isDaemon("S17", sectionsOf(ctx.env)) ? answer.chats : [];
  return {
    query: answer.query,
    projects: answer.projects.slice(0, SearchHitsPerKind).map((hit) => projectRow(ctx, hit)),
    cards: answer.cards.slice(0, SearchHitsPerKind).map((hit) => cardRow(ctx, hit)),
    chats: chats.slice(0, SearchHitsPerKind).map((hit) => chatRow(ctx, hit)),
  };
}

/**
 * The client to search with, or null when the palette must search the store instead: S24a is not
 * switched, there is no daemon, or the daemon is not answering right now.
 */
function searchApi(ctx: Ctx): ApiClient | null {
  const api = ctx.env.data?.api;
  if (!api || !isDaemon("S24a", sectionsOf(ctx.env))) return null;
  return ctx.S.connection?.state === "online" ? api : null;
}

/** True for a query the daemon would refuse for its length. It counts characters, not bytes. */
const tooLong = (query: string): boolean => [...query].length > MaxSearchQueryChars;

/**
 * Starts one search session for the palette. It keeps the newest query typed and sends it once
 * typing pauses, so a burst of keys is one request. An earlier request is cancelled when a newer
 * query is typed, and an answer that arrives anyway is dropped unless it answers the newest query:
 * the answer says which query it searched, and only the newest is shown. A request that fails
 * leaves no answer, and the palette then shows what the store holds. Stopping the session (or
 * disposing the component that made it) cancels what is waiting.
 */
export function createPaletteSearch(ctx: Ctx): PaletteSearch {
  const [hits, setHits] = createSignal<PaletteHits | null>(null);
  let newest = "";
  let round = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let inFlight: AbortController | undefined;

  const stop = (): void => {
    round += 1;
    clearTimeout(timer);
    timer = undefined;
    inFlight?.abort();
    inFlight = undefined;
  };

  async function send(api: ApiClient, query: string): Promise<void> {
    const mine = round;
    const request = new AbortController();
    inFlight = request;
    try {
      const answer = await api.search(query, { signal: request.signal });
      if (mine === round && answer.query === newest) setHits(hitsOf(ctx, answer));
    } catch {
      // Nothing to show for this query: the palette falls back to the store, which is what it
      // showed while it waited. A request that was cancelled is not this one any more.
      if (mine === round) setHits(null);
    } finally {
      if (inFlight === request) inFlight = undefined;
    }
  }

  const ask = (text: string): void => {
    const query = cleanQuery(text);
    if (query === newest) return;
    newest = query;
    stop();
    const api = searchApi(ctx);
    if (!query || !api || tooLong(query)) {
      setHits(null);
      return;
    }
    timer = setTimeout(() => void send(api, query), SEARCH_DEBOUNCE_MS);
  };

  if (getOwner()) onCleanup(stop);
  return { hits, ask, stop };
}
