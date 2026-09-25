import type { Marshal } from "~/mock";
import {
  AGENTS_STEP,
  CONTROL_STEP,
  PROFILE_STEP,
  PROJECT_STEP,
  SAMPLE_PROJECT,
} from "./onboarding-data";
import {
  isMonorepoHint,
  maskKey,
  type OnboardingDraft,
  repoNameOf,
  SHORT_KEY_LENGTH,
} from "./onboarding-draft";

/** The part of the store a step writes to. */
type Store = Pick<Marshal, "S" | "addProject">;

function commitProfile(m: Store, draft: OnboardingDraft): void {
  const { profile } = m.S;
  Object.assign(profile, {
    name: draft.name.trim() || profile.name,
    email: draft.email.trim(),
    tz: draft.tz,
  });
}

function commitKeys(m: Store, draft: OnboardingDraft): void {
  for (const [id, key] of Object.entries(draft.keys)) {
    const provider = m.S.providers.find((p) => p.id === id);
    if (!provider || key.length <= SHORT_KEY_LENGTH) continue;
    provider.st = "saved";
    provider.masked = maskKey(key);
  }
}

function commitProject(m: Store, draft: OnboardingDraft): void {
  if (draft.source === "sample") {
    if (m.S.projects.some((p) => p.name === SAMPLE_PROJECT.name)) return;
    m.addProject({ ...SAMPLE_PROJECT, sample: true });
    return;
  }
  const value = draft.source === "folder" ? draft.path : draft.url;
  const name = repoNameOf(value);
  if (!value.trim() || !name) return;
  m.addProject({
    name,
    path: draft.source === "folder" ? value : `~/code/${name}`,
    mono: isMonorepoHint(value),
  });
}

function commitChatApps(m: Store, draft: OnboardingDraft): void {
  for (const [id, connected] of Object.entries(draft.chatApps)) {
    const integration = m.S.integrations.find((x) => x.id === id);
    if (connected && integration) integration.st = "connected";
  }
}

/** Applies what one screen collected to the store. Runs on Continue only, never on Skip or Back. */
export function commitStep(step: number, draft: OnboardingDraft, m: Store): void {
  if (step === PROFILE_STEP) commitProfile(m, draft);
  else if (step === AGENTS_STEP) commitKeys(m, draft);
  else if (step === PROJECT_STEP) commitProject(m, draft);
  else if (step === CONTROL_STEP) commitChatApps(m, draft);
}
