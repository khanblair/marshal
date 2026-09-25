// biome-ignore-all lint/style/noMagicNumbers: seed data table, values are the prototype's fake data
import type { CalEvent, Schedule } from "../settings-types";

const WEEKDAYS = [1, 2, 3, 4, 5];

const SCHEDULES: Schedule[] = [
  {
    id: "s1",
    name: "Morning brief",
    kind: "brief",
    icon: "sunrise",
    trigger: "Cron",
    when: "Every weekday at 8:00",
    time: "08:00",
    days: WEEKDAYS,
    action: "Send the brief to the app, Telegram, and Obsidian",
    project: "All projects",
    enabled: true,
    missed: "Run once on wake",
  },
  {
    id: "s2",
    name: "Evening brief",
    kind: "brief",
    icon: "sunset",
    trigger: "Cron",
    when: "Every weekday at 18:00",
    time: "18:00",
    days: WEEKDAYS,
    action: "Send the brief to the app and Obsidian",
    project: "All projects",
    enabled: true,
    missed: "Skip",
  },
  {
    id: "s3",
    name: "Check open issues",
    kind: "job",
    icon: "clock",
    trigger: "Cron",
    when: "Every weekday at 9:00",
    time: "09:00",
    days: WEEKDAYS,
    action: "Send a message to the Orchestrator",
    project: "web-dashboard",
    enabled: true,
    missed: "Run once on wake",
  },
  {
    id: "s4",
    name: "Dependency updates",
    kind: "job",
    icon: "clock",
    trigger: "Cron",
    when: "Every Monday at 2:00",
    time: "02:00",
    days: [1],
    action: "Create a card from the Dependency update template",
    project: "api-gateway",
    enabled: true,
    missed: "Run once on wake",
  },
  {
    id: "s5",
    name: "Fix until e2e passes",
    kind: "job",
    icon: "repeat",
    trigger: "Interval",
    when: "Every 30 minutes",
    time: "",
    days: [],
    action: "Loop on #213, max 6 rounds, 2 hours, $4.00",
    project: "mobile-app",
    enabled: true,
    missed: "Skip",
  },
];

const CAL_EVENTS: CalEvent[] = [
  { id: "e1", title: "Standup", time: "09:30", days: WEEKDAYS },
  { id: "e2", title: "Design review", time: "14:00", dayOffset: 1 },
  { id: "e3", title: "Release planning", time: "11:00", dayOffset: 6 },
  { id: "e4", title: "Dentist", time: "16:30", dayOffset: -3 },
];

/* Cloned per store, so no two instances share an array. */
export const seedSchedules = (): Schedule[] => structuredClone(SCHEDULES);
export const seedCalEvents = (): CalEvent[] => structuredClone(CAL_EVENTS);
