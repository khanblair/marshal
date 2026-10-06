import type { SaveScheduleRequest, ScheduleCatalog } from "@marshal/protocol";
import { createMemo, createSignal } from "solid-js";
import type { Schedule } from "~/mock";
import { requestFrom } from "~/sync/schedule-actions";
import {
  briefAction,
  type IntervalUnit,
  normalDays,
  readInterval,
  whenForDays,
  whenForInterval,
} from "./schedule-words";

export const TRIGGERS = ["Cron", "Interval", "One-time", "Event"];
export const MISSED_RUNS = ["Run once on wake", "Skip"];

const DEFAULT_TIME = "09:00";
const DEFAULT_EVERY = 4;
/**
 * What a typed time must start with for a trigger, an example of it, and what to say when it does not.
 * Cron and Interval have pickers, so only One-time and Event are typed.
 */
export const WORDS: Record<
  string,
  { start: RegExp; example: string; hint: string; error: string }
> = {
  Event: {
    start: /^when\b/i,
    example: "When 30 minutes before my first calendar event",
    hint: 'For example "When 30 minutes before my first calendar event", or "When a calendar event named "Standup" starts".',
    error:
      'An Event schedule starts with "When", for example "When 30 minutes before my first calendar event".',
  },
  "One-time": {
    start: /^on\b/i,
    example: "On 1 October at 14:00",
    hint: 'For example "On 1 October at 14:00".',
    error: 'A One-time schedule starts with "On", for example "On 1 October at 14:00".',
  },
};

const INTERVAL_ERROR = "Give a whole number of minutes or hours, 1 or more.";

/**
 * What the schedule editor holds while it is open: one signal for each thing a person can change, and
 * the sentence and the save request made from them. Cron and Interval schedules write their words from
 * the time, days, and interval pickers, so the sentence, the time, and the days always agree. Other
 * triggers keep the words as typed.
 */
export function createScheduleForm(schedule: Schedule) {
  const interval = readInterval(schedule.when);
  const [name, setName] = createSignal(schedule.name);
  const [trigger, setTriggerOnly] = createSignal(schedule.trigger);
  const [missed, setMissed] = createSignal(schedule.missed);
  const [time, setTime] = createSignal(schedule.time || DEFAULT_TIME);
  const [days, setDays] = createSignal<number[]>(normalDays(schedule.days));
  const [every, setEvery] = createSignal(interval?.every ?? DEFAULT_EVERY);
  const [unit, setUnit] = createSignal<IntervalUnit>(interval?.unit ?? "hours");
  const [whenText, setWhenText] = createSignal(schedule.when);
  const [sections, setSections] = createSignal<string[]>(schedule.sections ?? []);
  const [deliver, setDeliver] = createSignal<string[]>(schedule.deliver ?? []);
  const [quiet, setQuiet] = createSignal(schedule.quietWhenEmpty ?? false);
  const [action, setAction] = createSignal(schedule.action);

  /** Changes the trigger, and gives a typed one an example when the words it has would never be read. */
  const setTrigger = (next: string): void => {
    setTriggerOnly(next);
    const words = WORDS[next];
    if (words && !words.start.test(whenText().trim())) setWhenText(words.example);
  };

  const when = createMemo(() => {
    if (trigger() === "Cron") return whenForDays(days(), time());
    if (trigger() === "Interval") return whenForInterval(every(), unit());
    return whenText().trim();
  });

  /** What is wrong with the form, in a sentence, or an empty string when it can be saved. */
  const problem = (): string => {
    if (trigger() === "Interval" && !(Number.isInteger(every()) && every() >= 1)) {
      return INTERVAL_ERROR;
    }
    const words = WORDS[trigger()];
    return words && !words.start.test(when()) ? words.error : "";
  };

  const isBrief = schedule.kind === "brief";

  /** The body to save, with everything the editor does not show carried over from the row. */
  const request = (catalog: ScheduleCatalog | null): SaveScheduleRequest =>
    requestFrom(schedule, {
      name: name().trim() || schedule.name,
      trigger: trigger(),
      when: when(),
      time: trigger() === "Cron" ? time() : schedule.time,
      days: trigger() === "Cron" ? days() : schedule.days,
      missed: missed(),
      sections: isBrief ? sections() : (schedule.sections ?? []),
      deliver: isBrief ? deliver() : (schedule.deliver ?? []),
      quietWhenEmpty: quiet(),
      action:
        isBrief && catalog
          ? briefAction(sections(), deliver(), catalog)
          : action().trim() || schedule.action,
    });

  return {
    isBrief,
    name,
    setName,
    trigger,
    setTrigger,
    missed,
    setMissed,
    time,
    setTime,
    days,
    setDays,
    every,
    setEvery,
    unit,
    setUnit,
    whenText,
    setWhenText,
    sections,
    setSections,
    deliver,
    setDeliver,
    quiet,
    setQuiet,
    action,
    setAction,
    when,
    problem,
    request,
  };
}

export type ScheduleForm = ReturnType<typeof createScheduleForm>;
