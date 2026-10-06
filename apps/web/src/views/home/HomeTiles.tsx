import { breakpoints } from "@marshal/tokens/tokens";
import { SummaryTile } from "@marshal/ui";
import { M } from "~/mock";
import { mergedTodayCount, openCostLimits, openStatusList } from "./home-actions";

/** The four numbers at the top of Home, two by two below the md band; each opens the cards or the limits behind it. */
export function HomeTiles() {
  const needs = () => M.needs().length;
  const cost = () => M.costs();
  // A daily ceiling of 0 means none is set: the tile then shows the spend alone, in the plain tone.
  const ceiling = () => cost().day > 0;
  const costTone = () => {
    if (!ceiling()) return "default";
    const tone = M.costTone(cost().today, cost().day);
    if (tone === "over") return "danger";
    return tone === "near" ? "needs-you" : "default";
  };
  return (
    <div class={`grid gap-3 ${M.S.vw < breakpoints.md ? "grid-cols-2" : "grid-cols-4"}`}>
      <SummaryTile
        value={needs()}
        label="Need you"
        title="Open the cards that need you"
        tone={needs() ? "needs-you" : "default"}
        onClick={() => openStatusList("needs")}
      />
      <SummaryTile
        value={M.working().length}
        label="Working now"
        title="Open working cards"
        onClick={() => openStatusList("working")}
      />
      <SummaryTile
        value={mergedTodayCount()}
        label="Merged today"
        title="Open done cards"
        onClick={() => openStatusList("done")}
      />
      <SummaryTile
        value={M.money(cost().today)}
        label={ceiling() ? `Cost today of ${M.money(cost().day)}` : "Cost today"}
        title="Open cost limits"
        tone={costTone()}
        onClick={openCostLimits}
      />
    </div>
  );
}
