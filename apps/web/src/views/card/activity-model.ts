import { type Activity, type ActivityKind, M } from "~/mock";

/** A row of the Activity tab. Colors are classes, chosen from the activity's state. */
export interface ActivityRow {
  id: string;
  icon: string;
  text: string;
  mono: boolean;
  result: string;
  resultIcon: string;
  resultClass: string;
  when: string;
  full: string;
  fresh: boolean;
}

const KIND_ICONS: Record<ActivityKind, string> = {
  file: "file-pen",
  command: "terminal",
  test: "flask-conical",
  tool: "wrench",
  approval: "st-needs",
};

/** The Activity tab shows the newest entries only. */
const ACTIVITY_LIMIT = 120;
/** An entry that is newer than this is highlighted for a moment. */
const FRESH_MS = 1500;
const FILE_VERBS = /^Ran |^Edited |^Read |^Created /;

function resultLook(act: Activity): { icon: string; class: string } {
  let icon = "check";
  if (act.st === "running") icon = "spinner";
  else if (act.st === "waiting") icon = "st-needs";
  else if (act.st === "fail") icon = "x";
  let cls = "text-status-working-text";
  if (act.st === "fail") cls = "text-status-danger-text";
  else if (act.st === "waiting") cls = "text-status-needs-you-text";
  else if (act.st === "running") cls = "text-secondary";
  return { icon, class: cls };
}

export function activityRows(acts: readonly Activity[]): ActivityRow[] {
  const now = M.now();
  return acts.slice(0, ACTIVITY_LIMIT).map((act) => {
    const look = resultLook(act);
    return {
      id: act.id,
      icon: KIND_ICONS[act.kind] ?? "dot",
      text: act.text,
      mono: act.kind === "command" || FILE_VERBS.test(act.text),
      result: act.result,
      resultIcon: look.icon,
      resultClass: look.class,
      when: M.rel(act.ts),
      full: M.full(act.ts),
      fresh: now - act.ts < FRESH_MS,
    };
  });
}

export interface Checkpoint {
  label: string;
  ref: string;
  when: string;
  restore: () => void;
}

const RECENT_CHECKPOINT_MIN = 12;
const MIDDLE_CHECKPOINT_MIN = 38;
const FIRST_CHECKPOINT_MIN = 95;
const CHECKPOINT_AGES_MIN = [
  RECENT_CHECKPOINT_MIN,
  MIDDLE_CHECKPOINT_MIN,
  FIRST_CHECKPOINT_MIN,
] as const;

/** Three fake restore points per started card, newest first. A ref is named by the card's number, because each project is its own repository. */
export function checkpointsFor(
  cardNumber: number,
  started: boolean,
  firstFile: string,
): Checkpoint[] {
  if (!started) return [];
  const labels = ["Before turn 6", `Before editing ${firstFile.split("/").pop()}`, "Session start"];
  const total = labels.length;
  return labels.map((label, i) => ({
    label,
    ref: `refs/marshal/cp/${cardNumber}/${total - i}`,
    when: `${CHECKPOINT_AGES_MIN[i]} min ago`,
    restore: () =>
      M.confirm({
        title: "Restore checkpoint",
        message: `This resets the worktree to "${label}". Newer changes are kept on a backup ref, so you can undo this.`,
        action: "Restore checkpoint",
        run: () => M.toast("Checkpoint restored"),
      }),
  }));
}
