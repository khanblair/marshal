import { cx } from "@marshal/ui";
import { onMount } from "solid-js";
import { M } from "~/mock";
import { createEditState } from "./edit-state";
import { SectionContent, type SettingsDrafts } from "./SectionContent";
import { SettingsNav } from "./SettingsNav";
import { createLimitsDraft } from "./use-limits-draft";
import { createProfileDraft } from "./use-profile-draft";
import { createProjectDraft } from "./use-project-draft";
import { createRoleDraft } from "./use-role-draft";

/**
 * Settings: a section list and the chosen section's page. Every form's unsaved edits live here,
 * not in the sections, so they survive switching between sections, as they do in the design.
 */
export function SettingsView() {
  const drafts: SettingsDrafts = {
    profile: createProfileDraft(),
    project: createProjectDraft(),
    roles: createRoleDraft(),
    limits: createLimitsDraft(),
    providerEdit: createEditState(),
    scheduleEdit: createEditState(),
  };
  // Another part of the app can ask for one schedule's editor to be open on arrival.
  onMount(() => {
    const requested = M.S.schedEdit;
    if (!requested) return;
    drafts.scheduleEdit.open(requested);
    M.S.schedEdit = null;
  });
  return (
    <div data-no-nav="1" class={cx("absolute inset-0 flex bg-canvas", M.mobile && "flex-col")}>
      <SettingsNav />
      <div class="flex-1 min-w-0 overflow-auto">
        <div class={cx("max-w-[880px] flex flex-col gap-6", M.mobile ? "p-4" : "pt-6 px-8 pb-12")}>
          <SectionContent drafts={drafts} />
        </div>
      </div>
    </div>
  );
}
