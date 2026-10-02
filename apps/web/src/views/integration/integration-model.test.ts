import type { IntegrationState } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { toMergeFlow } from "~/data/mappers/integration";
import { golden } from "~/data/testing/golden";
import type { Card, Chat } from "~/mock/types";
import {
  aheadLabel,
  integratorChatOf,
  nothingWaiting,
  stoppedCards,
  storeCardOf,
} from "./integration-model";

const wire = golden<IntegrationState>("integration-state");

/** A store card with only the fields the model reads. */
const card = (fields: Partial<Card> & Pick<Card, "id" | "p">): Card =>
  ({ state: "working", ...fields }) as Card;

describe("stoppedCards", () => {
  const conflict = card({ id: "web#14", p: "web", state: "needs", reasonKind: "conflict" });
  const ciFailed = card({ id: "web#15", p: "web", state: "needs", reasonKind: "ci-failed" });
  const question = card({ id: "web#16", p: "web", state: "needs", reasonKind: "question" });
  const elsewhere = card({ id: "api#1", p: "api", state: "needs", reasonKind: "conflict" });
  const cards = [conflict, ciFailed, question, elsewhere, card({ id: "web#17", p: "web" })];

  it("lists the merge conflicts of the project, and nothing a CI failure or a question left", () => {
    const flow = toMergeFlow({ ...wire, state: "idle", queue: [], history: [] });
    expect(stoppedCards(cards, "web", flow)).toEqual([conflict]);
  });

  it("adds the card the Integrator says it is waiting on, whatever the reason", () => {
    const waitingOn = card({
      id: "web#16",
      daemonId: "01HZ",
      p: "web",
      state: "needs",
      reasonKind: "question",
    });
    const waiting = toMergeFlow({ ...wire, state: "waiting", currentCardId: "01HZ" });
    expect(stoppedCards([conflict, waitingOn], "web", waiting)).toEqual([conflict, waitingOn]);
    const merging = toMergeFlow({ ...wire, state: "merging", currentCardId: "01HZ" });
    expect(stoppedCards([conflict, waitingOn], "web", merging)).toEqual([conflict]);
  });

  it("does not list a card that has left Needs you", () => {
    const retried = card({ id: "web#14", p: "web", state: "merging", reasonKind: "conflict" });
    expect(stoppedCards([retried], "web", toMergeFlow(wire))).toEqual([]);
  });
});

describe("nothingWaiting", () => {
  it("is true only when the queue is empty and no card has stopped", () => {
    const idle = toMergeFlow({ ...wire, queue: [], history: [] });
    expect(nothingWaiting(idle, [])).toBe(true);
    expect(nothingWaiting(idle, [card({ id: "web#14", p: "web" })])).toBe(false);
    expect(nothingWaiting(toMergeFlow(wire), [])).toBe(false);
  });
});

describe("storeCardOf", () => {
  it("finds a card by the daemon's id inside its project", () => {
    const here = card({ id: "web#12", daemonId: "01HZ", p: "web" });
    const other = card({ id: "api#12", daemonId: "01HZ", p: "api" });
    expect(storeCardOf([other, here], "web", "01HZ")).toBe(here);
    expect(storeCardOf([here], "web", "nope")).toBeUndefined();
    expect(storeCardOf([card({ id: "web#9", p: "web" })], "web", "")).toBeUndefined();
  });
});

describe("aheadLabel", () => {
  it("says how far ahead the Integrator is, or that it is up to date", () => {
    expect(aheadLabel(0)).toBe("Up to date");
    expect(aheadLabel(2)).toBe("Ahead by 2");
  });
});

describe("integratorChatOf", () => {
  const chat = (id: string, system?: string) => ({ id, system }) as unknown as Chat;

  it("finds the chat the store marks as the Integrator's, and none when it does not", () => {
    const integrator = chat("c2", "integrator");
    expect(integratorChatOf([chat("c1"), integrator])).toBe(integrator);
    expect(integratorChatOf([chat("c1")])).toBeUndefined();
    expect(integratorChatOf(undefined)).toBeUndefined();
  });
});
