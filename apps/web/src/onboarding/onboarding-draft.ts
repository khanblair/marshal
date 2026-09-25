import type { ChatAppId, KeyId, ProjectSource } from "./onboarding-data";

/** Everything typed or chosen on the five screens. It is only applied to the store on Continue. */
export interface OnboardingDraft {
  name: string;
  email: string;
  tz: string;
  avatarChosen: boolean;
  keys: Record<KeyId, string>;
  source: ProjectSource;
  path: string;
  url: string;
  chatApps: Record<ChatAppId, boolean>;
}

export function initialDraft(): OnboardingDraft {
  return {
    name: "",
    email: "",
    tz: "Europe/London",
    avatarChosen: false,
    keys: { anthropic: "", openai: "", gemini: "" },
    source: "sample",
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

/** Keys of this many characters or fewer are treated as not entered. */
export const SHORT_KEY_LENGTH = 8;
const KEY_HEAD_CHARS = 6;
const KEY_TAIL_CHARS = 4;

/** How a saved key is shown: the first six characters, an ellipsis, the last four. */
export function maskKey(key: string): string {
  return `${key.slice(0, KEY_HEAD_CHARS)}…${key.slice(-KEY_TAIL_CHARS)}`;
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

/** Names that hint at a monorepo. */
export const isMonorepoHint = (value: string): boolean => /mono|apps/i.test(value);
