import type { CardKey } from "../card-key";
import { MINUTE_MS } from "../constants";
import { type IdCounters, takeCk } from "../ids";
import type { Attachment, Card, Checklist, Comment, Person } from "../types";

export const PEOPLE: Person[] = [
  { id: "ada", name: "Ada Okafor" },
  { id: "blair", name: "Blair Akandwanaho" },
  { id: "godana", name: "Godana Emiru" },
  { id: "angella", name: "Angella Nsubuga" },
];

interface ChecklistSeed {
  title: string;
  items: string[];
  done: number;
  by?: string;
}

interface CommentSeed {
  author: string;
  text: string;
  ago: number;
  att?: Attachment[];
}

interface CardExtraSeed {
  id: CardKey;
  members: string[];
  checklists?: ChecklistSeed[];
  comments?: CommentSeed[];
}

/** Checked items were done 17 minutes apart, the last one 17 minutes before load. */
const ITEM_SPACING_MIN = 17;

/* Listed in the prototype's order, which decides the seeded checklist and comment ids. */
const EXTRAS: CardExtraSeed[] = [
  {
    id: "api#41",
    members: ["ada", "blair"],
    checklists: [
      {
        title: "Sub-tasks",
        items: [
          "Find why two refreshes run at once",
          "Add a single-flight guard to the refresh client",
          "Retry the request once after refresh",
          "Run the race detector on the auth package",
        ],
        done: 3,
      },
      {
        title: "Definition of done",
        items: [
          "No logouts when a token expires mid-request",
          "Tests cover concurrent refresh",
          "Reviewer approved the pull request",
        ],
        done: 1,
        by: "blair",
      },
    ],
    comments: [
      {
        author: "blair",
        text: "Repro steps from support: log in, leave the tab for 16 minutes, then open two reports at once. Screenshot of the logout attached.",
        ago: 180,
        att: [{ kind: "image", name: "logout-after-refresh.png", size: "184 KB" }],
      },
      {
        author: "agent",
        text: "Read this before starting. The two parallel report requests explain the double refresh. Covered by the concurrent refresh test.",
        ago: 150,
      },
      {
        author: "ada",
        text: "Related Sentry issue: https://sentry.io/issues/48213 and the auth spec in the wiki.",
        ago: 60,
        att: [
          { kind: "link", name: "sentry.io/issues/48213", url: "https://sentry.io/issues/48213" },
          { kind: "file", name: "auth-refresh-spec.pdf", size: "312 KB" },
        ],
      },
    ],
  },
  {
    id: "api#43",
    members: ["ada", "godana"],
    checklists: [
      {
        title: "Acceptance criteria",
        items: [
          "Each API key has its own limit",
          "Limits come from the plan tier",
          "Over-limit requests get 429 with Retry-After",
          "Limiter adds less than 200 ns per call",
        ],
        done: 0,
      },
    ],
    comments: [
      {
        author: "godana",
        text: "Enterprise keys should get 1,000 requests per minute, not 100. Pricing sheet attached.",
        ago: 40,
        att: [{ kind: "file", name: "plan-tiers-2026.xlsx", size: "48 KB" }],
      },
    ],
  },
  { id: "api#44", members: ["ada"] },
  {
    id: "web#118",
    members: ["angella", "ada"],
    checklists: [
      {
        title: "Sub-tasks",
        items: [
          "Replace hard-coded grays with tokens",
          "Check contrast in dark mode",
          "Capture before and after screenshots",
        ],
        done: 1,
      },
    ],
    comments: [
      {
        author: "angella",
        text: "Design reference for the dark settings page is in Figma. Keep the section dividers.",
        ago: 25,
        att: [
          {
            kind: "link",
            name: "figma.com/file/settings-dark",
            url: "https://figma.com/file/settings-dark",
          },
          { kind: "image", name: "settings-dark-mock.png", size: "402 KB" },
        ],
      },
    ],
  },
  {
    id: "web#119",
    members: ["godana"],
    comments: [
      {
        author: "godana",
        text: "Type the column helper with createColumnHelper<ReportRow>(). The loose typing caused bugs last time.",
        ago: 8,
      },
    ],
  },
  { id: "mobile#209", members: ["blair"] },
  {
    id: "mobile#207",
    members: ["angella"],
    checklists: [
      {
        title: "Variants",
        items: ["Primary", "Secondary", "Ghost", "Destructive", "Icon only"],
        done: 5,
      },
    ],
  },
  {
    id: "mobile#210",
    members: ["ada", "godana"],
    checklists: [
      {
        title: "Definition of done",
        items: [
          "Requests queue while offline",
          "Queue replays in order when back online",
          "Errors use ApiClientError",
        ],
        done: 2,
      },
    ],
  },
  { id: "web#116", members: ["godana"] },
  { id: "api#39", members: ["blair"] },
  { id: "api#36", members: ["ada"] },
];

function makeChecklist(ids: IdCounters, loadedAt: number, seed: ChecklistSeed): Checklist {
  const id = `cl${takeCk(ids)}`;
  const count = seed.items.length;
  const items = seed.items.map((text, i) => ({
    id: `it${takeCk(ids)}`,
    text,
    done: i < seed.done,
    by: i < seed.done ? seed.by || "agent" : null,
    doneAt: loadedAt - (count - i) * ITEM_SPACING_MIN * MINUTE_MS,
  }));
  return { id, title: seed.title, hideDone: false, items };
}

function makeComment(ids: IdCounters, loadedAt: number, seed: CommentSeed): Comment {
  return {
    id: `co${takeCk(ids)}`,
    author: seed.author,
    text: seed.text,
    ts: loadedAt - seed.ago * MINUTE_MS,
    att: structuredClone(seed.att || []),
    read: true,
  };
}

/** Adds members, checklists, and comments to the seeded cards. */
export function seedCardExtras(cards: Card[], ids: IdCounters, loadedAt: number): void {
  for (const extra of EXTRAS) {
    const card = cards.find((c) => c.id === extra.id);
    if (!card) continue;
    card.members = [...extra.members];
    if (extra.checklists)
      card.checklists = extra.checklists.map((s) => makeChecklist(ids, loadedAt, s));
    if (extra.comments) card.comments = extra.comments.map((s) => makeComment(ids, loadedAt, s));
  }
}
