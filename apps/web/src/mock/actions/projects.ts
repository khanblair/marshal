import type { Ctx } from "../context";
import { feed, toast } from "../engine";
import { takeMid } from "../ids";
import { activityFrom, genericChat } from "../seed/generic";
import { card, proj } from "../selectors";
import type { Notice, Status } from "../types";
import { insertCard, newCardFrom } from "./card-create";

export interface NewProject {
  name: string;
  path: string;
  branch?: string;
  lang?: string;
  mono?: boolean;
  sample?: boolean;
}

const MONO_PACKAGES = ["apps/web", "packages/core", "packages/ui"];
const NEW_PROJECT_LIMITS = { day: 8, month: 120, awake: 6 };

/** The three cards of the sample project; cost grows by this much per card. */
const SAMPLE_CARDS: { title: string; state: Status }[] = [
  { title: "Add a health check endpoint", state: "backlog" },
  { title: "Fix the typo in the README", state: "working" },
  { title: "Write tests for the date helper", state: "review" },
];
const SAMPLE_COST_STEP_USD = 0.12;
const SAMPLE_UPD_MIN = 5;
const SAMPLE_SPAN_DAYS = 2;
const SAMPLE_PR = 12;

function addSampleCards(ctx: Ctx, pid: string): void {
  SAMPLE_CARDS.forEach(({ title, state }, i) => {
    const c = newCardFrom(ctx, {
      p: pid,
      title,
      state,
      branch: state !== "backlog" ? `marshal/sample-${i + 1}` : null,
      ci: state === "review" ? "passed" : null,
      cost: state === "backlog" ? 0 : SAMPLE_COST_STEP_USD * (i + 1),
      doing: state === "working" ? "Editing README.md" : "",
      s: i,
      e: i + SAMPLE_SPAN_DAYS,
      upd: SAMPLE_UPD_MIN,
      pr: state === "review" ? SAMPLE_PR : null,
    });
    const chat = genericChat(c, ctx.msg);
    insertCard(ctx, c, chat, activityFrom(chat, c.upd, ctx.ids));
  });
}

/** Adds a project with default views and limits. Returns its id. */
export function addProject(ctx: Ctx, o: NewProject): string {
  const { S } = ctx;
  const id = `p${takeMid(ctx.ids)}`;
  const mono = !!o.mono;
  S.projects.push({
    id,
    name: o.name,
    lang: mono ? "Monorepo" : o.lang || "TypeScript",
    path: o.path,
    branch: o.branch || "main",
    ci: "queued",
    ciAgo: 0,
    monthBase: 0,
    packages: mono ? [...MONO_PACKAGES] : undefined,
    runs: [{ wf: "ci", st: "queued", ago: 0 }],
  });
  S.filters[id] = [];
  S.query[id] = "";
  S.swim[id] = mono ? "package" : "none";
  S.savedViews[id] = [{ name: "All cards", f: [], swim: "none" }];
  S.savedView[id] = "All cards";
  S.lastView[id] = "board";
  S.chats[id] = [];
  S.limits[id] = { ...NEW_PROJECT_LIMITS };
  if (o.sample) addSampleCards(ctx, id);
  feed(ctx, { kind: "tool", text: `${o.name} was added to Marshal`, pid: id });
  return id;
}

export function renameProject(ctx: Ctx, id: string, name: string): boolean {
  const p = proj(ctx, id);
  if (!name?.trim()) {
    toast(ctx, "Project names can't be empty. The old name is kept.");
    return false;
  }
  if (!p) return false;
  p.name = name.trim();
  return true;
}

/** A notice stays after a project is removed unless it belongs to it or only points at removed cards. */
function noticeSurvives(ctx: Ctx, n: Notice, pid: string): boolean {
  if (n.kind === "sleep") return n.cards.length > 0;
  if (n.pid === pid) return false;
  return !(n.cardId && !card(ctx, n.cardId));
}

/** Removes a project with its cards and chats. The repository on disk is not touched. */
export function removeProject(ctx: Ctx, id: string): void {
  const { S } = ctx;
  const p = proj(ctx, id);
  S.cards = S.cards.filter((c) => c.p !== id);
  delete S.chats[id];
  S.projects = S.projects.filter((x) => x.id !== id);
  for (const n of S.notices) {
    if (n.kind === "sleep") n.cards = n.cards.filter((cid) => card(ctx, cid));
  }
  S.notices = S.notices.filter((n) => noticeSurvives(ctx, n, id));
  if (S.openId && !card(ctx, S.openId)) S.openId = null;
  if (S.route.pid === id) {
    S.route = { page: "home", pid: S.projects[0]?.id ?? null, view: "board" };
  }
  feed(ctx, {
    kind: "tool",
    text: `${p?.name} was removed from Marshal. The repository on disk was not touched.`,
    pid: null,
  });
  toast(ctx, "Project removed");
}
