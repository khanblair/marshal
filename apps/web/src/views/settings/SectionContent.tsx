import { Match, Switch } from "solid-js";
import { M } from "~/mock";
import type { EditState } from "./edit-state";
import { GeneralSection } from "./GeneralSection";
import { HelpSection } from "./HelpSection";
import { IntegrationsSection } from "./IntegrationsSection";
import { LimitsSection } from "./LimitsSection";
import { ProfileSection } from "./ProfileSection";
import { ProjectSection } from "./ProjectSection";
import { ProvidersSection } from "./ProvidersSection";
import { RolesSection } from "./RolesSection";
import { SchedulesSection } from "./SchedulesSection";
import { ShortcutsSection } from "./ShortcutsSection";
import type { SectionKey } from "./sections";
import type { LimitsDraft } from "./use-limits-draft";
import type { ProfileDraft } from "./use-profile-draft";
import type { ProjectDraft } from "./use-project-draft";
import type { RoleDraft } from "./use-role-draft";

export interface SettingsDrafts {
  profile: ProfileDraft;
  project: ProjectDraft;
  roles: RoleDraft;
  limits: LimitsDraft;
  providerEdit: EditState;
  scheduleEdit: EditState;
}

/** The page for the chosen section. An unknown section draws nothing, as in the design. */
export function SectionContent(props: { drafts: SettingsDrafts }) {
  const is = (key: SectionKey) => M.S.settingsSection === key;
  return (
    <Switch>
      <Match when={is("profile")}>
        <ProfileSection profile={props.drafts.profile} />
      </Match>
      <Match when={is("project")}>
        <ProjectSection project={props.drafts.project} />
      </Match>
      <Match when={is("help")}>
        <HelpSection />
      </Match>
      <Match when={is("general")}>
        <GeneralSection />
      </Match>
      <Match when={is("roles")}>
        <RolesSection draft={props.drafts.roles} />
      </Match>
      <Match when={is("providers")}>
        <ProvidersSection edit={props.drafts.providerEdit} />
      </Match>
      <Match when={is("limits")}>
        <LimitsSection draft={props.drafts.limits} />
      </Match>
      <Match when={is("schedules")}>
        <SchedulesSection edit={props.drafts.scheduleEdit} />
      </Match>
      <Match when={is("integrations")}>
        <IntegrationsSection />
      </Match>
      <Match when={is("shortcuts")}>
        <ShortcutsSection />
      </Match>
    </Switch>
  );
}
