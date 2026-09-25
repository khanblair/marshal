import type { Ctx } from "../context";
import { later, set } from "../engine";
import { card, isMobile } from "../selectors";
import type { CardTab, Mode, Page, ViewKey } from "../types";

/** Opens a page. For "project", the view defaults to the one last used in that project. */
export function go(ctx: Ctx, page: Page, pid?: string | null, view?: ViewKey): void {
  const { S } = ctx;
  const route = { ...S.route, page };
  if (pid) route.pid = pid;
  if (page === "project") {
    route.view = view || (route.pid && S.lastView[route.pid]) || "board";
    if (route.pid) S.lastView[route.pid] = route.view;
  }
  S.route = route;
  S.menu = null;
  S.noticesOpen = false;
  if (page === "settings") {
    S.openId = null;
    S.detailExpanded = false;
  }
  if (isMobile(S)) {
    S.openId = null;
    if (page === "home") S.mobileTab = "home";
    else S.mobileTab = route.view === "chat" ? "chat" : "board";
  }
}

export function setView(ctx: Ctx, view: ViewKey): void {
  const { S } = ctx;
  if (S.route.page !== "project") S.route = { ...S.route, page: "project" };
  S.route = { ...S.route, view };
  if (S.route.pid) S.lastView[S.route.pid] = view;
  S.menu = null;
}

/** Opens a card in the detail panel, switching project when the board shows another one. */
export function openCard(ctx: Ctx, id: number | string, tab?: CardTab): void {
  const { S } = ctx;
  const c = card(ctx, id);
  if (!c) return;
  S.openId = c.id;
  S.focusId = c.id;
  S.tab = tab || "chat";
  S.mode = "chat";
  S.switching = false;
  S.menu = null;
  S.noticesOpen = false;
  S.palette = false;
  if (S.route.page === "project" && S.route.pid !== c.p) {
    S.route = { ...S.route, pid: c.p, view: S.lastView[c.p] ?? S.route.view };
  }
}

export function closeCard(ctx: Ctx): void {
  set(ctx, { openId: null, detailExpanded: false });
}

export function setTab(ctx: Ctx, tab: CardTab): void {
  set(ctx, { tab });
}

/** Time the fake agent session takes to switch between chat and terminal. */
const MODE_SWITCH_MS = 1400;

export function setMode(ctx: Ctx, mode: Mode): void {
  const { S } = ctx;
  if (mode === S.mode || S.switching) return;
  set(ctx, { switching: mode });
  later(MODE_SWITCH_MS, () => set(ctx, { mode, switching: false }));
}

export function setViewport(ctx: Ctx, w: number, h: number): void {
  const { S } = ctx;
  if (S.vw === w && S.vh === h) return;
  S.vw = w;
  S.vh = h;
}
