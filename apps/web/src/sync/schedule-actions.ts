import type { SaveScheduleRequest, ScheduleCatalog, ScheduleRun } from "@marshal/protocol";
import { type ScheduleRow, toScheduleRow } from "~/data/mappers/schedules";
import type { Ctx } from "~/mock/context";
import { proj } from "~/mock/selectors";

/*
 * The writes the Schedules screen makes on the daemon (section S30, B8.1). A save answers the one
 * row it changed, not a whole list, so each of these applies its own answer onto the store's row
 * by id rather than reloading everything.
 */

function projectName(ctx: Ctx): (id: string) => string {
  return (id) => proj(ctx, id)?.name ?? id;
}

/** Adds a schedule. The daemon makes its id and answers the row as stored. */
export async function createSchedule(
  ctx: Ctx,
  body: SaveScheduleRequest,
): Promise<ScheduleRow | null> {
  const api = ctx.env.data?.api;
  if (!api) return null;
  try {
    const created = await ctx.optimistic({
      apply: () => undefined,
      request: () => api.createSchedule(body),
      rollback: () => undefined,
    });
    const row = toScheduleRow(created, projectName(ctx));
    ctx.S.schedules.push(row);
    return row;
  } catch {
    return null;
  }
}

/** Edits one schedule and applies the daemon's own answer onto the store's row. */
export async function saveSchedule(
  ctx: Ctx,
  id: string,
  body: SaveScheduleRequest,
): Promise<boolean> {
  const api = ctx.env.data?.api;
  if (!api) return false;
  const index = ctx.S.schedules.findIndex((row) => row.id === id);
  const current = ctx.S.schedules[index];
  if (!current) return false;
  const previous = { ...current };
  try {
    const saved = await ctx.optimistic({
      key: `schedule:${id}`,
      apply: () => undefined,
      request: () => api.saveSchedule(id, body),
      rollback: () => {
        ctx.S.schedules[index] = previous;
      },
    });
    ctx.S.schedules[index] = toScheduleRow(saved, projectName(ctx));
    return true;
  } catch {
    return false;
  }
}

/** Turns a stored row back into a save request, with changes applied on top. */
export function requestFrom(
  row: ScheduleRow,
  changes: Partial<SaveScheduleRequest> = {},
): SaveScheduleRequest {
  return {
    id: row.id,
    project: row.projectId,
    name: row.name,
    kind: row.kind,
    icon: row.icon,
    trigger: row.trigger,
    when: row.when,
    time: row.time,
    days: row.days,
    action: row.action,
    enabled: row.enabled,
    missed: row.missed,
    template: row.template ?? "",
    sections: row.sections ?? [],
    deliver: row.deliver ?? [],
    quietWhenEmpty: row.quietWhenEmpty ?? false,
    ...changes,
  };
}

/** Removes a schedule and its run history. */
export async function deleteSchedule(ctx: Ctx, id: string): Promise<boolean> {
  const api = ctx.env.data?.api;
  if (!api) return false;
  const index = ctx.S.schedules.findIndex((row) => row.id === id);
  const removed = ctx.S.schedules[index];
  if (!removed) return false;
  try {
    await ctx.optimistic({
      key: `schedule-del:${id}`,
      apply: () => {
        ctx.S.schedules.splice(index, 1);
      },
      request: () => api.deleteSchedule(id),
      rollback: () => {
        ctx.S.schedules.splice(index, 0, removed);
      },
    });
    return true;
  } catch {
    return false;
  }
}

/** A schedule's own run history, newest first - a brief's own composed text is a run's details. */
export async function scheduleRuns(ctx: Ctx, id: string): Promise<ScheduleRun[]> {
  const api = ctx.env.data?.api;
  if (!api) return [];
  try {
    return (await api.scheduleRuns(id)).runs;
  } catch {
    return [];
  }
}

const catalogs = new WeakMap<Ctx, Promise<ScheduleCatalog | null>>();

/**
 * What the editor offers: the starter templates, the parts of a brief, and the chats it can go to. The
 * daemon's own list, read once and kept, because it only changes with a new version. Null when there is
 * no daemon or it could not be read, and then the next call asks again.
 */
export function scheduleCatalog(ctx: Ctx): Promise<ScheduleCatalog | null> {
  const api = ctx.env.data?.api;
  if (!api) return Promise.resolve(null);
  const kept = catalogs.get(ctx);
  if (kept) return kept;
  const asked = api.scheduleCatalog().catch(() => {
    catalogs.delete(ctx);
    return null;
  });
  catalogs.set(ctx, asked);
  return asked;
}

/** Runs one schedule now, and answers the run it recorded, or null when it could not be run. */
export async function runSchedule(ctx: Ctx, id: string): Promise<ScheduleRun | null> {
  const api = ctx.env.data?.api;
  if (!api) return null;
  try {
    return await api.runSchedule(id);
  } catch {
    return null;
  }
}

/** The message a chat would get from a brief now, or null when it cannot be previewed. Nothing is sent. */
export async function previewSchedule(ctx: Ctx, id: string): Promise<string | null> {
  const api = ctx.env.data?.api;
  if (!api) return null;
  try {
    return (await api.previewSchedule(id)).text;
  } catch {
    return null;
  }
}
