import { currentTimeZone } from "~/data/time-zones";
import { type ChatAppId, DEFAULT_PROJECT_SOURCE, type ProjectSource } from "./onboarding-data";

/** Everything typed or chosen on the five screens. It is only applied to the store on Continue. */
export interface OnboardingDraft {
  name: string;
  email: string;
  tz: string;
  source: ProjectSource;
  path: string;
  url: string;
  chatApps: Record<ChatAppId, boolean>;
}

export function initialDraft(): OnboardingDraft {
  return {
    name: "",
    email: "",
    tz: currentTimeZone(),
    source: DEFAULT_PROJECT_SOURCE,
    path: "",
    url: "",
    chatApps: { telegram: false, discord: false },
  };
}

const MAX_INITIALS = 2;

/** Up to two capital letters from the first letters of each word; `?` while the name is empty. */
export function initialsOf(name: string): string {
  return (name.trim() || "?")
    .split(/\s+/)
    .map((word) => word.charAt(0))
    .join("")
    .slice(0, MAX_INITIALS)
    .toUpperCase();
}

/** The last part of a repository path or URL, without `.git`; empty when there is none. */
export function repoNameOf(value: string): string {
  return (
    value
      .replace(/\.git$/, "")
      .split(/[/:]/)
      .filter(Boolean)
      .pop() ?? ""
  );
}
