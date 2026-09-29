import type { ScheduleList } from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import { toScheduleRows } from "~/data/mappers/schedules";
import { proj } from "~/mock/selectors";
import type { Syncer } from "./syncer";

/**
 * The scheduled briefs and jobs (section S30, B8.1, build-plan 8.1). One call answers every
 * schedule; there is no per-project list here because the screen already reads a project's own
 * from the whole one.
 */
export const schedulesSyncer: Syncer<ScheduleList> = {
  section: "S30",
  topics: [],
  async load(api: ApiClient) {
    return api.listSchedules();
  },
  apply(ctx, list) {
    ctx.S.schedules = toScheduleRows(list, (id) => proj(ctx, id)?.name ?? id);
  },
};
