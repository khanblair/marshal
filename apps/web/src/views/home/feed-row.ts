import { batch } from "solid-js";
import { type FeedItem, M } from "~/mock";

/* How one entry of the activity feed looks and what it opens. */

export interface FeedIcon {
  name: string;
  color: string;
}

const QUIET = "var(--color-text-secondary)";
const FAILED = /failed/i;

/** The icon and its color of a feed entry. CI results are red or green by their text. */
export function feedIcon(item: FeedItem): FeedIcon {
  switch (item.kind) {
    case "merge":
      return { name: "git-merge", color: M.tone("ready", "solid") };
    case "ci":
      return FAILED.test(item.text)
        ? { name: "circle-x", color: M.tone("danger", "solid") }
        : { name: "circle-check", color: M.tone("working", "solid") };
    case "approval":
      return { name: "st-needs", color: M.tone("needs-you", "solid") };
    case "plan":
      return { name: "list-checks", color: QUIET };
    case "schedule":
      return { name: "clock", color: QUIET };
    case "brief":
      return { name: "sunrise", color: QUIET };
    default:
      return { name: "info", color: QUIET };
  }
}

/** The name of the entry's project, or nothing when it has none or the project is gone. */
export const feedProjectName = (item: FeedItem): string =>
  (item.pid && M.proj(item.pid)?.name) || "";

/** Opens what the entry is about: its card, else its schedule, else its project. */
export function openFeedItem(item: FeedItem): void {
  if (item.cardId && M.card(item.cardId)) {
    M.openCard(item.cardId);
  } else if (item.job) {
    const job = item.job;
    batch(() => {
      M.S.settingsSection = "schedules";
      M.S.schedEdit = job;
    });
    M.go("settings");
  } else if (item.pid && M.proj(item.pid)) {
    M.go("project", item.pid);
  }
}
