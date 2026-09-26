import type { CreateProjectRequest } from "@marshal/protocol";
import type { Marshal } from "~/mock";
import { AGENTS_STEP, CONTROL_STEP, PROFILE_STEP, PROJECT_STEP } from "./onboarding-data";
import { maskKey, type OnboardingDraft, repoNameOf, SHORT_KEY_LENGTH } from "./onboarding-draft";

/** The part of the store a step writes to. */
type Store = Pick<Marshal, "S" | "addProject" | "toast">;

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

/** Adds the first project on the daemon. A refusal shows as a toast and setup carries on. */
async function addFirstProject(m: Store, request: CreateProjectRequest): Promise<void> {
  const result = await m.addProject(request);
  if ("error" in result) m.toast(result.error);
}

/**
 * Adds the project the person named, from a folder or a URL, or the sample. An empty field adds
 * nothing. The sample is the daemon's own repository: the request names only the source, and the
 * daemon answers with the sample it already has (`409 conflict`) when it is in Marshal.
 */
function commitProject(m: Store, draft: OnboardingDraft): void {
  if (draft.source === "sample") {
    void addFirstProject(m, { source: "sample" });
    return;
  }
  const value = (draft.source === "folder" ? draft.path : draft.url).trim();
  const name = repoNameOf(value);
  if (!value || !name) return;
  void addFirstProject(
    m,
    draft.source === "folder"
      ? { source: "folder", path: value, name }
      : { source: "clone", url: value, dest: `~/code/${name}`, name },
  );
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
