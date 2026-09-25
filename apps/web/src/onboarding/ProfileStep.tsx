import { Avatar, Button, Field, Input, SegmentedControl, Select } from "@marshal/ui";
import { M } from "~/mock";
import { THEME_OPTIONS, TIME_ZONES } from "./onboarding-data";
import { initialsOf } from "./onboarding-draft";
import { StepIntro } from "./StepIntro";
import type { StepProps } from "./StepProps";

export interface ProfileStepProps extends StepProps {
  /** Show "Enter a name to continue." under the name field. */
  nameError: boolean;
}

/** Screen 2: name, email, time zone, and theme. Only the theme is applied right away. */
export function ProfileStep(props: ProfileStepProps) {
  const chooseAvatar = () => {
    props.setDraft("avatarChosen", true);
    M.toast("Choose an image to use as your avatar");
  };
  return (
    <>
      <StepIntro>
        This is how you appear on cards and in briefs. You can change it later in your profile.
      </StepIntro>
      <div class="flex flex-wrap items-center gap-4">
        <Avatar
          size={64}
          bordered
          aria-label="Avatar preview"
          initials={initialsOf(props.draft.name)}
        />
        <div class="flex flex-col gap-1">
          <Button class="self-start hover:bg-surface!" onClick={chooseAvatar}>
            {props.draft.avatarChosen ? "Change image" : "Upload image"}
          </Button>
          <span class="text-small text-secondary">
            Without an image, Marshal shows your initials.
          </span>
        </div>
      </div>
      <div class="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-4">
        <Field label="Name" error={props.nameError ? "Enter a name to continue." : undefined}>
          <Input
            value={props.draft.name}
            onInput={(e) => props.setDraft("name", e.currentTarget.value)}
            autocomplete="name"
          />
        </Field>
        <Field label="Email" hint="Used only to send briefs by email.">
          <Input
            type="email"
            value={props.draft.email}
            onInput={(e) => props.setDraft("email", e.currentTarget.value)}
            placeholder="Optional"
            autocomplete="email"
          />
        </Field>
        <Field label="Time zone" hint="Briefs and schedules run in this time zone.">
          <Select
            class="px-2.5!"
            options={TIME_ZONES}
            value={props.draft.tz}
            onChange={(e) => props.setDraft("tz", e.currentTarget.value)}
          />
        </Field>
      </div>
      <div class="flex flex-col gap-1.5">
        <span class="font-medium">Theme</span>
        <SegmentedControl
          class="self-start"
          label="Theme"
          options={THEME_OPTIONS}
          value={M.S.theme}
          onValueChange={(theme) => M.setTheme(theme)}
          size={30}
          unselectedTone="primary"
          segmentClass="px-3!"
        />
      </div>
    </>
  );
}
