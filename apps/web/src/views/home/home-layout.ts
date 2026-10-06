import { M } from "~/mock";

/* Window widths, from `M.S.vw`, at which Home changes its layout. */

/** From here the lists sit in two columns. */
const TWO_COLUMN_MIN_WIDTH_PX = 900;
/** From here the two charts sit side by side. */
const CHART_COLUMNS_MIN_WIDTH_PX = 1000;

export const twoColumns = (): boolean => M.S.vw >= TWO_COLUMN_MIN_WIDTH_PX;
export const chartColumns = (): boolean => M.S.vw >= CHART_COLUMNS_MIN_WIDTH_PX;

/** The padding of both Home pages. */
export const pagePadding = (): string => (M.mobile ? "px-4 pt-4 pb-8" : "px-6 pt-6 pb-12");
