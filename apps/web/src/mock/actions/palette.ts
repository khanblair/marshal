import { batch } from "solid-js";
import { STATUS, tone, VIEWS } from "../constants";
import type { Ctx } from "../context";
import { set, toast } from "../engine";
import { card, pendingApproval, proj } from "../selectors";
import { approve } from "./approvals";
import { newCard } from "./card-create";
import { fork } from "./cards";
import { go, openCard, setView } from "./navigation";
import { startTour } from "./onboarding";
import { pin, sleep, wake } from "./sessions";
import { setTheme, simulateCiFailure } from "./settings";

export interface Command {
  group: string;
  label: string;
  icon: string;
  kbd?: string[];
  hint?: string;
  iconColor?: string;
  run: () => void;
}

const MERGE_START_PCT = 10;
const CI_DEMO_CARD_ID = 40;

const SETTINGS: [section: string, label: string][] = [
  ["profile", "Profile"],
  ["project", "Project settings"],
  ["help", "Help"],
  ["general", "Theme"],
  ["roles", "Roles"],
  ["providers", "Provider keys"],
  ["limits", "Cost and awake limits"],
  ["schedules", "Schedules"],
  ["integrations", "Integrations"],
  ["shortcuts", "Keyboard shortcuts"],
];

/** Command runs change several fields at once, so each applies as one update. */
const cmd = (c: Command): Command => ({ ...c, run: () => batch(c.run) });

function generalCommands(ctx: Ctx): Command[] {
  const { S } = ctx;
  const pid = S.route.pid;
  return [
    {
      group: "Actions",
      label: "New card",
      icon: "plus",
      kbd: ["N"],
      run: () => {
        if (S.route.page !== "project") go(ctx, "project", pid);
        newCard(ctx);
      },
    },
    ...VIEWS.map((v, i) => ({
      group: "Actions",
      label: `Switch to ${v.label.toLowerCase()} view`,
      icon: v.icon,
      kbd: ["⌘", String(i + 1)],
      run: () => {
        if (S.route.page !== "project") go(ctx, "project", pid, v.key);
        else setView(ctx, v.key);
      },
    })),
    { group: "Actions", label: "Go home", icon: "home", run: () => go(ctx, "home") },
    {
      group: "Actions",
      label: "New project",
      icon: "folder-plus",
      run: () =>
        set(ctx, { newProject: { source: "folder", path: "", url: "", name: "", branch: "main" } }),
    },
    {
      group: "Actions",
      label: "New chat",
      icon: "message-square-plus",
      run: () => {
        go(ctx, "project", pid, "chat");
        set(ctx, { newChatOpen: true });
      },
    },
    { group: "Actions", label: "Replay tour", icon: "map", run: () => startTour(ctx) },
  ];
}

function openCardCommands(ctx: Ctx): Command[] {
  const c = card(ctx, ctx.S.openId);
  if (!c) return [];
  const out: Command[] = [
    {
      group: "Actions",
      label: `${c.pinned ? "Unpin" : "Pin"} #${c.id}`,
      icon: "pin",
      kbd: ["P"],
      run: () => pin(ctx, c.id),
    },
    {
      group: "Actions",
      label: c.asleep ? `Resume session on #${c.id}` : `Sleep #${c.id}`,
      icon: "moon",
      kbd: ["S"],
      run: () => (c.asleep ? wake(ctx, c.id) : sleep(ctx, c.id)),
    },
    { group: "Actions", label: `Fork #${c.id}`, icon: "git-fork", run: () => fork(ctx, c.id) },
  ];
  if (pendingApproval(ctx, c.id)) {
    out.push({
      group: "Actions",
      label: `Approve on #${c.id}`,
      icon: "check",
      kbd: ["A"],
      run: () => approve(ctx, c.id),
    });
  }
  return out;
}

function projectCommands(ctx: Ctx): Command[] {
  const { S } = ctx;
  const out: Command[] = [];
  const rm = S.cards.find((c) => c.state === "ready" && c.p === S.route.pid);
  if (rm) {
    out.push({
      group: "Actions",
      label: `Merge #${rm.id} ${rm.title}`,
      icon: "git-merge",
      run: () => {
        rm.state = "merging";
        rm.mergePct = MERGE_START_PCT;
        rm.doing = "Dry-run merge with git merge-tree";
        toast(ctx, "Added to merge queue");
      },
    });
  }
  out.push(
    {
      group: "Actions",
      label: `Simulate CI failure on #${CI_DEMO_CARD_ID}`,
      icon: "circle-x",
      run: () => simulateCiFailure(ctx, CI_DEMO_CARD_ID),
    },
    {
      group: "Actions",
      label: "Show notices",
      icon: "bell",
      run: () => set(ctx, { noticesOpen: true }),
    },
  );
  for (const p of S.projects) {
    out.push({
      group: "Projects",
      label: p.name,
      icon: "folder-git-2",
      hint: p.lang,
      run: () => go(ctx, "project", p.id),
    });
  }
  return out;
}

function cardAndSettingsCommands(ctx: Ctx): Command[] {
  const { S } = ctx;
  const cards: Command[] = S.cards.map((c) => ({
    group: "Cards",
    label: `#${c.id} ${c.title}`,
    icon: STATUS[c.state].icon,
    iconColor: tone(STATUS[c.state].tone, "solid"),
    hint: proj(ctx, c.p)?.name ?? "",
    run: () => openCard(ctx, c.id),
  }));
  const settings: Command[] = SETTINGS.map(([section, label]) => ({
    group: "Settings",
    label,
    icon: "settings",
    run: () => {
      S.settingsSection = section;
      go(ctx, "settings");
    },
  }));
  const themes: Command[] = [
    { group: "Settings", label: "Use light theme", icon: "sun", run: () => setTheme(ctx, "light") },
    { group: "Settings", label: "Use dark theme", icon: "moon", run: () => setTheme(ctx, "dark") },
    {
      group: "Settings",
      label: "Use system theme",
      icon: "monitor",
      run: () => setTheme(ctx, "system"),
    },
  ];
  return [...cards, ...settings, ...themes];
}

/** Everything the command palette can run, in the prototype's order. */
export function commands(ctx: Ctx): Command[] {
  return [
    ...generalCommands(ctx),
    ...openCardCommands(ctx),
    ...projectCommands(ctx),
    ...cardAndSettingsCommands(ctx),
  ].map(cmd);
}
