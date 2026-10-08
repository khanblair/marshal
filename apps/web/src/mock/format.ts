import { relTime } from "~/data/format";
import { formatDateTime } from "~/data/zone";

const CENTS_PER_DOLLAR = 100;

export const money = (v: number): string =>
  `$${(Math.round(v * CENTS_PER_DOLLAR) / CENTS_PER_DOLLAR).toFixed(2)}`;

/* `relTime` lives in `~/data/format` and is re-exported here so the mock's own modules keep one
   import. The dependency points from the mock to the data layer, never the other way, because the
   mock is deleted in Phase 13. */
export { relTime };

export const full = (ts: number): string => formatDateTime(ts);
