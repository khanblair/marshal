import { createEffect, createSignal } from "solid-js";
import { go, openCard } from "~/mock/actions/navigation";
import { newCard } from "~/mock/actions/card-create";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";
import { type Platform, platform } from "~/platform";

/*
 * The links that open the app at one place (docs/mobile.md section 3): `marshal://card/<id>` and
 * `marshal://approval/<id>?card=<id>` from a notice, `marshal://share?text=...` from "Share to
 * Marshal", and the same card link as an address on the daemon's own page (`?open=card/<id>`), which
 * is what a chat message can carry where a custom scheme is not a link. Ids are the daemon's own.
 */

export type DeepLink =
  | { kind: "card"; cardId: string }
  | { kind: "share"; title: string; body: string };

const TITLE_LIMIT = 80;

/** A card's title from shared text: its first line, cut to a length that fits a card. */
function titleOf(text: string): string {
  const first = text.split("\n").find((line) => line.trim()) ?? "";
  return first.trim().slice(0, TITLE_LIMIT);
}

function cardLink(id: string | null | undefined): DeepLink | null {
  return id ? { kind: "card", cardId: id } : null;
}

function shareLink(params: URLSearchParams): DeepLink | null {
  const text = (params.get("text") ?? "").trim();
  if (!text) return null;
  return { kind: "share", title: (params.get("title") ?? "").trim() || titleOf(text), body: text };
}

/** Reads a link. Anything that is not one of Marshal's is null, so it is never acted on. */
export function resolveDeepLink(link: string): DeepLink | null {
  let url: URL;
  try {
    url = new URL(link);
  } catch {
    return null;
  }
  if (url.protocol === "marshal:") {
    const id = decodeURIComponent(url.pathname.replace(/^\//, ""));
    if (url.hostname === "card") return cardLink(id);
    if (url.hostname === "approval") return cardLink(url.searchParams.get("card"));
    return url.hostname === "share" ? shareLink(url.searchParams) : null;
  }
  const open = url.searchParams.get("open") ?? "";
  return open.startsWith("card/") ? cardLink(open.slice("card/".length)) : null;
}

/** Shows what a link points at. False when the card it names is not in the store. */
export function applyDeepLink(ctx: Ctx, link: DeepLink): boolean {
  const { S } = ctx;
  if (link.kind === "card") {
    const card = S.cards.find((one) => one.daemonId === link.cardId);
    if (!card) {
      toast(ctx, "That card is not here. It may have been removed.");
      return false;
    }
    openCard(ctx, card.id);
    return true;
  }
  const project = S.route.pid ?? S.projects[0]?.id;
  if (!project) {
    toast(ctx, "Add a project first, then share again.");
    return false;
  }
  go(ctx, "project", project, "board");
  newCard(ctx, { title: link.title, body: link.body });
  return true;
}

/**
 * Follows the links that open the app: the ones the device hands over, and the one this page was
 * opened with. A link that arrives before the first data is kept and shown once the app is ready,
 * because a cold start from a notice has no cards yet.
 */
export function followDeepLinks(ctx: Ctx, device: Platform = platform()): () => void {
  const [waiting, setWaiting] = createSignal<DeepLink[]>([]);
  const take = (url: string) => {
    const link = resolveDeepLink(url);
    if (link) setWaiting((all) => [...all, link]);
  };
  const stop = device.onDeepLink(take);
  const params = new URLSearchParams(window.location.search);
  if (params.has("open")) {
    take(window.location.href);
    params.delete("open");
    const rest = params.toString();
    window.history.replaceState(null, "", window.location.pathname + (rest ? `?${rest}` : ""));
  }
  createEffect(() => {
    const links = waiting();
    if (!ctx.S.ready || links.length === 0) return;
    setWaiting([]);
    for (const link of links) applyDeepLink(ctx, link);
  });
  return stop;
}
