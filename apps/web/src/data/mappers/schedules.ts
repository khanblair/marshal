import type { Schedule as WireSchedule, ScheduleList } from "@marshal/protocol";

/**
 * The words the screen shows for a schedule that covers every project - a brief, usually - rather
 * than one. It matches the mock's own seed data exactly, so the screen's existing "All projects"
 * checks keep working once this is real.
 */
export const ALL_PROJECTS = "All projects";

/**
 * One schedule as the Schedules screen draws it: the daemon's own fields, with `project` turned
 * from an id into the name a person reads, done here rather than in the pure mapper below so this
 * file never depends on the store.
 */
export interface ScheduleRow {
  id: string;
  name: string;
  kind: "brief" | "job";
  icon: string;
  trigger: string;
  when: string;
  time: string;
  days: number[];
  action: string;
  project: string;
  /** The project's own id, empty for a schedule that covers every project. What a save sends back. */
  projectId: string;
  enabled: boolean;
  missed: string;
}

/** One schedule, with its project id turned into the name projectName resolves it to. */
export function toScheduleRow(
  wire: WireSchedule,
  projectName: (id: string) => string,
): ScheduleRow {
  return {
    id: wire.id,
    name: wire.name,
    kind: wire.kind === "brief" ? "brief" : "job",
    icon: wire.icon,
    trigger: wire.trigger,
    when: wire.when,
    time: wire.time,
    days: wire.days,
    action: wire.action,
    project: wire.project ? projectName(wire.project) : ALL_PROJECTS,
    projectId: wire.project,
    enabled: wire.enabled,
    missed: wire.missed,
  };
}

/** Every schedule in a list answer, each with its project name resolved. */
export function toScheduleRows(
  list: ScheduleList,
  projectName: (id: string) => string,
): ScheduleRow[] {
  return list.schedules.map((wire) => toScheduleRow(wire, projectName));
}
