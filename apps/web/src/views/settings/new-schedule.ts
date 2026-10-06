import type { SaveScheduleRequest, ScheduleCatalog, ScheduleTemplate } from "@marshal/protocol";
import { briefAction, WEEKDAYS, whenForDays } from "./schedule-words";

/** The parts a schedule made from nothing starts with: the board as it stands. */
const CUSTOM_SECTIONS = ["needs-you", "working", "in-review", "finished"];
const CUSTOM_TIME = "09:00";

const channelIds = (catalog: ScheduleCatalog): string[] => catalog.channels.map((one) => one.id);

/**
 * A schedule from a starter template, switched off, going to every chat the catalog lists. A chat that is
 * not set up is skipped when the brief goes out, so ticking them all loses nothing.
 */
export function fromTemplate(
  template: ScheduleTemplate,
  catalog: ScheduleCatalog,
): SaveScheduleRequest {
  return {
    project: "",
    name: template.name,
    kind: "brief",
    icon: template.icon,
    trigger: template.trigger,
    when: template.when,
    time: template.time,
    days: template.days,
    action: template.summary,
    enabled: false,
    missed: template.missed,
    template: template.key,
    sections: template.sections,
    deliver: channelIds(catalog),
    quietWhenEmpty: template.quietWhenEmpty,
  };
}

/**
 * A schedule made from nothing: a brief of the board's usual parts, every weekday at 9:00, switched off.
 * Without a catalog (a store with no daemon) it has no parts, and is a brief made the old way.
 */
export function custom(catalog: ScheduleCatalog | null): SaveScheduleRequest {
  const sections = catalog ? CUSTOM_SECTIONS : [];
  const deliver = catalog ? channelIds(catalog) : [];
  return {
    project: "",
    name: "New schedule",
    kind: "brief",
    icon: "clock",
    trigger: "Cron",
    when: whenForDays(WEEKDAYS, CUSTOM_TIME),
    time: CUSTOM_TIME,
    days: [...WEEKDAYS],
    action: catalog ? briefAction(sections, deliver, catalog) : "",
    enabled: false,
    missed: "Skip",
    template: "",
    sections,
    deliver,
    quietWhenEmpty: false,
  };
}
