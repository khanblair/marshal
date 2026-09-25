/**
 * A fixture for the card key tests: two projects that each have a card number 12, so any place
 * that mixes up a card's number and its key shows the wrong card. The projects arrive through the
 * mirror, as the daemon's would, and the cards through `quickAdd`, so the numbers come from the
 * per-project counter and the seed is not touched.
 */

import { applyProject, applyProjectRemoved } from "~/sync/projects";
import { daemonProject } from "~/testing/projects";
import { contextOf } from "~/testing/test-store";
import { type CardKey, cardKey } from "../card-key";
import type { Marshal } from "../marshal";

/** The number both fixture cards have. */
const SHARED_NUMBER = 12;

export interface TwoProjects {
  /** The project ids. */
  alpha: string;
  beta: string;
  /** `<alpha>#12` and `<beta>#12`. */
  alphaCard: CardKey;
  betaCard: CardKey;
}

const ALPHA_TITLE = "Alpha login form";
const BETA_TITLE = "Beta billing export";

function addProjectWithCards(M: Marshal, name: string, title: string): string {
  const pid = name;
  applyProject(contextOf(M), daemonProject({ id: pid, path: `~/code/${name}` }));
  // Each new project starts at 1, so the twelfth card is number 12 whatever else exists.
  for (let n = 1; n < SHARED_NUMBER; n++) M.quickAdd(pid, "backlog", `${name} filler ${n}`);
  M.quickAdd(pid, "backlog", title);
  return pid;
}

/** Adds alpha-service and beta-service to the store, each with cards 1 to 12 in Backlog. */
export function addTwoProjects(M: Marshal): TwoProjects {
  const alpha = addProjectWithCards(M, "alpha-service", ALPHA_TITLE);
  const beta = addProjectWithCards(M, "beta-service", BETA_TITLE);
  return {
    alpha,
    beta,
    alphaCard: cardKey(alpha, SHARED_NUMBER),
    betaCard: cardKey(beta, SHARED_NUMBER),
  };
}

/** Takes the fixture's projects out again, with their cards. */
export function removeTwoProjects(M: Marshal, two: TwoProjects): void {
  applyProjectRemoved(contextOf(M), two.alpha);
  applyProjectRemoved(contextOf(M), two.beta);
}
