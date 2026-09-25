import { type Column, M, type SwimKey } from "~/mock";

/** The column the phone shows when none was picked yet. */
const DEFAULT_MOBILE_COLUMN: Column = "working";

export const boardPid = (): string => M.S.route.pid ?? "";

/** Phones never draw lanes. The store may hold no value for a project (drawn as a lane "all"). */
export const boardSwim = (): SwimKey | undefined => (M.mobile ? "none" : M.S.swim[boardPid()]);

/** The phone's current column. */
export const mobileColumn = (): Column => M.S.mobileCol || DEFAULT_MOBILE_COLUMN;

/** The columns drawn: all seven, or the phone's one. */
export const boardColumns = (): readonly Column[] => (M.mobile ? [mobileColumn()] : M.COLUMNS);

/** Key of a lane in `S.laneCollapsed`. */
export const laneStoreKey = (pid: string, swim: SwimKey | undefined, laneKey: string): string =>
  `${pid}:${String(swim)}:${laneKey}`;
