/**
 * A fixture for the card key tests: two projects that each have a card number 12, so any place
 * that mixes up a card's number and its key shows the wrong card. The projects arrive through the
 * mirror, as the daemon's would, and the cards through `quickAdd`, so the numbers come from the
 * per-project counter and the seed is not touched.
 *
 * The cards are the daemon's now, so each project is put in both places it is needed: the store,
 * where the views read it, and the fake daemon, which is what the quick adds are written to.
 */

import { toDaemonProject } from "~/data/mappers/project";
import { applyProject, applyProjectRemoved } from "~/sync/projects";
import type { FakeDaemon } from "~/testing/fake-daemon";
import { wireProject } from "~/testing/projects";
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

/**
 * Adds one project to the store and to the daemon, then fills its board with cards 1 to 12 through
 * the daemon's own create route, so the numbers come from the daemon's per-project counter.
 */
async function addProjectWithCards(
  M: Marshal,
  d: FakeDaemon,
  name: string,
  title: string,
): Promise<string> {
  const project = wireProject({ id: name, path: `~/code/${name}` });
  applyProject(contextOf(M), toDaemonProject(project));
  d.projects.push(project);
  // Each new project starts empty, so the twelfth card is number 12 whatever else exists.
  for (let n = 1; n < SHARED_NUMBER; n++) {
    await M.quickAdd(name, "backlog", `${name} filler ${n}`);
  }
  await M.quickAdd(name, "backlog", title);
  return name;
}

/** Adds alpha-service and beta-service, each with cards 1 to 12 in Backlog. */
export async function addTwoProjects(M: Marshal, d: FakeDaemon): Promise<TwoProjects> {
  const alpha = await addProjectWithCards(M, d, "alpha-service", ALPHA_TITLE);
  const beta = await addProjectWithCards(M, d, "beta-service", BETA_TITLE);
  return {
    alpha,
    beta,
    alphaCard: cardKey(alpha, SHARED_NUMBER),
    betaCard: cardKey(beta, SHARED_NUMBER),
  };
}

/**
 * Takes the fixture's projects out again, with their cards, from the store and from the daemon, so
 * the next test starts with neither of them anywhere. The daemon's own arrays are spliced in place,
 * because its router answers from those arrays, not from copies.
 */
export function removeTwoProjects(M: Marshal, d: FakeDaemon, two: TwoProjects): void {
  applyProjectRemoved(contextOf(M), two.alpha);
  applyProjectRemoved(contextOf(M), two.beta);
  for (const pid of [two.alpha, two.beta]) {
    const at = d.projects.findIndex((project) => project.id === pid);
    if (at >= 0) d.projects.splice(at, 1);
  }
  const mine = (projectId: string): boolean => projectId === two.alpha || projectId === two.beta;
  d.cards.splice(0, d.cards.length, ...d.cards.filter((card) => !mine(card.projectId)));
}
