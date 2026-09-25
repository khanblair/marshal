import { Icon } from "@marshal/ui";
import { M } from "~/mock";
import { cardCountLabel, type LaneModel } from "./board-model";
import { boardPid, boardSwim, laneStoreKey } from "./board-state";

export interface LaneHeaderProps {
  lane: LaneModel;
}

/** The button above a swimlane that names it, counts its cards, and collapses it. */
export function LaneHeader(props: LaneHeaderProps) {
  const toggle = () => {
    const key = laneStoreKey(boardPid(), boardSwim(), props.lane.key);
    M.S.laneCollapsed[key] = !props.lane.collapsed;
  };
  const mono = () => boardSwim() === "package" || boardSwim() === "label";
  return (
    <button
      type="button"
      onClick={toggle}
      aria-expanded={!props.lane.collapsed}
      class="sticky left-0 self-start inline-flex items-center gap-2 h-8 mt-3 mb-1.5 px-2 border-none rounded-sm bg-transparent font-semibold hover:bg-surface-hover"
    >
      <Icon name={props.lane.collapsed ? "chevron-right" : "chevron-down"} />
      <span class={mono() ? "font-mono text-small" : undefined}>{props.lane.key}</span>
      <span class="font-normal text-small text-secondary">{cardCountLabel(props.lane.count)}</span>
    </button>
  );
}
