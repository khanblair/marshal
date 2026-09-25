import { For, Show } from "solid-js";
import { BoardColumn } from "./BoardColumn";
import type { LaneModel } from "./board-model";
import { boardColumns, boardSwim } from "./board-state";
import { LaneHeader } from "./LaneHeader";
import { columnOf } from "./use-board";

export interface BoardLaneProps {
  lane: LaneModel;
}

/** One swimlane: its header button (when the board has lanes) and a row of columns. */
export function BoardLane(props: BoardLaneProps) {
  return (
    <>
      <Show when={boardSwim() !== "none"}>
        <LaneHeader lane={props.lane} />
      </Show>
      <Show when={!props.lane.collapsed}>
        <div class="flex items-stretch gap-3">
          <For each={boardColumns()}>
            {(col) => <BoardColumn laneKey={props.lane.key} column={columnOf(props.lane, col)} />}
          </For>
        </div>
      </Show>
    </>
  );
}
