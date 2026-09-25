export interface TourStep {
  /**
   * `data-tour` values to look for, in order: the first one that is on screen with a size wins.
   * The second name is the phone version of the same thing.
   */
  targets: readonly string[];
  title: string;
  body: string;
}

/** The nine stops of the Home tour, in order. */
export const TOUR_STEPS: readonly [TourStep, ...TourStep[]] = [
  {
    targets: ["tiles"],
    title: "Summary and charts",
    body: "The tiles and charts show what is working, what merged, and what it costs against your limits.",
  },
  {
    targets: ["needs"],
    title: "Needs you",
    body: "Plans, approvals, conflicts, and failed checks that wait on you are listed here, oldest first.",
  },
  {
    targets: ["activity"],
    title: "Recent activity",
    body: "Merges, CI results, approvals, and schedule runs from every project, as they happen.",
  },
  {
    targets: ["projects", "projects-phone"],
    title: "Your projects",
    body: "Each project has one board. Badges show what needs you, main branch CI, and awake agents.",
  },
  {
    targets: ["new-project", "more-phone"],
    title: "Add a project",
    body: "Add a repository folder or clone one from GitHub. Monorepos are detected for you.",
  },
  {
    targets: ["views", "views-phone"],
    title: "Switch views",
    body: "Inside a project, move between board, chats, agents, list, timeline, and calendar.",
  },
  {
    targets: ["search", "more-phone"],
    title: "Search and shortcuts",
    body: "Press Cmd or Ctrl and K to search actions, projects, cards, and settings.",
  },
  {
    targets: ["notices"],
    title: "Notices",
    body: "Sleep warnings, CI failures, and cost warnings that happen while you are away.",
  },
  {
    targets: ["avatar"],
    title: "Your profile",
    body: "Open your profile and settings, or replay this tour.",
  },
];

/** The step at an index; an index out of range gives the first step instead of nothing. */
export function stepAt(index: number): TourStep {
  return TOUR_STEPS[index] ?? TOUR_STEPS[0];
}
