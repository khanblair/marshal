import { type Card, M } from "~/mock";

const STATE_ORDER: readonly string[] = [
  "working",
  "needs",
  "planning",
  "merging",
  "review",
  "ready",
];

/** Cards with a live agent session, the busiest states first. */
export const awakeCards = (): Card[] =>
  M.awake().sort((a, b) => STATE_ORDER.indexOf(a.state) - STATE_ORDER.indexOf(b.state));
