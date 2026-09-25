import type { CardKey } from "../card-key";
import type { Card, Check, CheckState, Status } from "../types";
import { lintFor, testFor } from "./files";

const REVIEWED: readonly Status[] = ["review", "ready", "merging", "done"];
const APPROVED: readonly Status[] = ["ready", "merging", "done"];
/** #119 is stuck on a type error, so its tests show as failed whatever its state. */
const STUCK_CARD_ID: CardKey = "web#119";

function overallState(c: Card): CheckState {
  if (c.state === "backlog") return "pending";
  if (REVIEWED.includes(c.state)) return "passed";
  if (c.ci === "failed") return "failed";
  return c.state === "working" ? "running" : "pending";
}

/** Acceptance checks for a card, derived from its state like the prototype's `checksFor`. */
export function checksFor(c: Card): Check[] {
  const st = overallState(c);
  const list: Check[] = [
    { id: "k1", name: "Tests pass", cmd: testFor(c), st },
    { id: "k2", name: "Lint clean", cmd: lintFor(c), st: st === "running" ? "passed" : st },
    {
      id: "k3",
      name: "Reviewer approval",
      cmd: "",
      st: APPROVED.includes(c.state) ? "passed" : "pending",
    },
  ];
  if (c.p === "web" && c.labels.includes("ui")) {
    list.push({
      id: "k4",
      name: "Screenshot matches",
      cmd: "marshal screenshot /settings",
      st: st === "passed" ? "passed" : "pending",
    });
  }
  const tests = list[0];
  if (c.id === STUCK_CARD_ID && tests) tests.st = "failed";
  return list;
}
