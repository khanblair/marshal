import { MINUTE_MS } from "../constants";
import { type IdCounters, takeMid } from "../ids";
import type { Activity, ActivityKind, Card, Msg, Status, ToolMsg } from "../types";
import { firstFile, testFor } from "./files";
import type { MsgFactory } from "./messages";

/* Fake sizes derived from the card id, so each card shows different numbers. */
const READ_LINES = { base: 80, spread: 120 };
const EDIT_ADDED = { base: 12, spread: 30 };
const EDIT_REMOVED_SPREAD = 9;
const TESTS_PASSED = { base: 20, spread: 60 };

const WITH_PR: readonly Status[] = ["review", "ready", "merging", "done"];
const APPROVED: readonly Status[] = ["ready", "merging", "done"];
const PLANNING_KEPT_MSGS = 3;

const lowerFirst = (s: string): string => s.charAt(0).toLowerCase() + s.slice(1);

/** The made-up chat history of a card, based on its state. */
export function genericChat(c: Card, b: MsgFactory): Msg[] {
  if (c.state === "backlog") return [];
  const file = firstFile(c);
  const out: Msg[] = [
    b.user(`Please ${lowerFirst(c.title)}. Keep the change small and add tests.`),
    b.agent("I'll read the relevant code first, then make the change."),
    b.tool("file-search", `Read ${file}`, `${READ_LINES.base + (c.n % READ_LINES.spread)} lines`),
    b.tool(
      "file-pen",
      `Edited ${file}`,
      `+${EDIT_ADDED.base + (c.n % EDIT_ADDED.spread)} −${c.n % EDIT_REMOVED_SPREAD}`,
    ),
    b.tool(
      "terminal",
      `Ran ${testFor(c)}`,
      `${TESTS_PASSED.base + (c.n % TESTS_PASSED.spread)} passed`,
    ),
  ];
  if (c.state === "planning") {
    return [
      ...out.slice(0, PLANNING_KEPT_MSGS),
      b.agent("I'm reading the callers before I write a plan."),
    ];
  }
  if (WITH_PR.includes(c.state)) {
    out.push(b.tool("git-pull-request", `Opened pull request #${c.pr}`, "Checks started"));
    out.push(b.agent(`I opened pull request #${c.pr}. The Reviewer is reading it now.`));
  }
  if (APPROVED.includes(c.state)) out.push(b.system("Reviewer approved. All checks passed."));
  if (c.state === "merging") {
    out.push(b.system("The Integrator picked up this card. Dry-run merge found no conflicts."));
  }
  if (c.state === "done") out.push(b.system("Merged into main. Worktree removed."));
  return out;
}

/* Seeded activity is spaced out after a start 20 minutes before the card's last update. */
const ACTIVITY_START_MIN = 20;
const TOOL_STEP_MIN = 3;
const APPROVAL_STEP_MIN = 2;
const SYSTEM_STEP_MIN = 1;

function toolKind(x: ToolMsg): ActivityKind {
  if (x.icon === "terminal") return /test|tsc/.test(x.action) ? "test" : "command";
  return x.icon === "git-pull-request" ? "tool" : "file";
}

/** Activity log derived from a chat, newest first. */
export function activityFrom(list: readonly Msg[], upd: number, ids: IdCounters): Activity[] {
  const items: Activity[] = [];
  let t = upd - ACTIVITY_START_MIN * MINUTE_MS;
  const add = (item: Omit<Activity, "id" | "ts">, stepMin: number): void => {
    t += stepMin * MINUTE_MS;
    items.push({ id: `a${takeMid(ids)}`, ...item, ts: t });
  };
  for (const x of list) {
    if (x.k === "tool") {
      add({ kind: toolKind(x), text: x.action, result: x.result, st: x.st }, TOOL_STEP_MIN);
    } else if (x.k === "approval") {
      const text = `Asked to run ${x.cmd}`;
      add({ kind: "approval", text, result: "Waiting", st: "waiting" }, APPROVAL_STEP_MIN);
    } else if (x.k === "system") {
      add({ kind: "tool", text: x.text, result: "", st: "ok" }, SYSTEM_STEP_MIN);
    }
  }
  return items.reverse();
}
