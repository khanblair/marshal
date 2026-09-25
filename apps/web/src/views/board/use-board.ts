import { createMemo, createRenderEffect, onCleanup } from "solid-js";
import { type Card, type Column, M } from "~/mock";
import {
  type BoardModel,
  buildBoard,
  type ColumnModel,
  countByColumn,
  type LaneModel,
} from "./board-model";
import { boardColumns, boardPid, boardSwim, laneStoreKey } from "./board-state";

const NAV_OWNER = "board";

const emptyLane = (key: string): LaneModel => ({ key, count: 0, collapsed: false, columns: [] });

/** The column model of a lane; a column the lane no longer draws reads as empty. */
export function columnOf(lane: LaneModel, col: Column): ColumnModel {
  return lane.columns.find((c) => c.col === col) ?? { col, cards: [], total: 0, showAll: false };
}

export interface Board {
  /** The project's cards after filters and search. */
  list: () => readonly Card[];
  model: () => BoardModel;
  /** Lane keys only, so the lane components stay mounted when the model is rebuilt. */
  laneKeys: () => string[];
  laneOf: (key: string) => LaneModel;
  counts: () => Record<Column, number>;
}

/**
 * The board's derived state. Lane and column components look their models up by key, so a store
 * change (a simulation tick) updates them in place instead of remounting them, which would drop
 * the focus of the quick add form.
 */
export function useBoard(): Board {
  const list = createMemo(() => M.filtered(boardPid()));
  const model = createMemo(() =>
    buildBoard({
      list: list(),
      swim: boardSwim(),
      packages: M.proj(boardPid())?.packages ?? [],
      columns: boardColumns(),
      isCollapsed: (laneKey) => !!M.S.laneCollapsed[laneStoreKey(boardPid(), boardSwim(), laneKey)],
      showAllDone: !!M.S.showAllDone[boardPid()],
    }),
  );
  const laneKeys = createMemo(() => model().lanes.map((lane) => lane.key));
  const lanesByKey = createMemo(() => new Map(model().lanes.map((lane) => [lane.key, lane])));
  // Arrow keys in the shell move the card focus through this grid.
  createRenderEffect(() => {
    M.nav = { owner: NAV_OWNER, grid: model().grid };
  });
  onCleanup(() => {
    if (M.nav?.owner === NAV_OWNER) M.nav = null;
  });
  return {
    list,
    model,
    laneKeys,
    laneOf: (key) => lanesByKey().get(key) ?? emptyLane(key),
    counts: createMemo(() => countByColumn(list())),
  };
}
