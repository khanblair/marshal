import { Avatar, Button, Field, Input, SegmentedControl, Select } from "@marshal/ui";
import { zoneChoices } from "~/data/time-zones";
import { M } from "~/mock";
import { THEME_OPTIONS } from "./onboarding-data";
import { initialsOf } from "./onboarding-draft";
import { StepIntro } from "./StepIntro";
import type { StepProps } from "./StepProps";

export interface ProfileStepProps extends StepProps {
  /** The sentence under the name field while the name is not acceptable. Empty when it is. */
  nameError: string;
  /** The sentence under the email field while a typed email is not acceptable. Empty when it is. */
  emailError: string;
}

/** Screen 2: name, email, time zone, and theme. Only the theme is applied right away. */
export function ProfileStep(props: ProfileStepProps) {
  // The image is saved as soon as it is picked, the same as in Settings; the daemon checks its type and size.
  const chooseAvatar = () => void M.chooseAvatar();
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
          initials={props.draft.name.trim() ? initialsOf(props.draft.name) : ""}
          src={M.S.profile.avatar ?? undefined}
        />
        <div class="flex flex-col gap-1">
          <Button class="self-start hover:bg-surface!" onClick={chooseAvatar}>
            {M.S.profile.avatar ? "Change image" : "Upload image"}
          </Button>
          <span class="text-small text-secondary">
            A PNG, JPEG, or WebP image. Without one, Marshal shows your initials.
          </span>
        </div>
      </div>
      <div class="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-4">
        <Field label="Name" required error={props.nameError || undefined}>
          <Input
            value={props.draft.name}
            required
            maxLength={100}
            onInput={(e) => props.setDraft("name", e.currentTarget.value)}
            autocomplete="name"
          />
        </Field>
        <Field
          label="Email"
          required
          hint="Used only to send briefs by email."
          error={props.emailError || undefined}
        >
          <Input
            type="email"
            value={props.draft.email}
            onInput={(e) => props.setDraft("email", e.currentTarget.value)}
            placeholder="you@example.com"
            required
            autocomplete="email"
          />
        </Field>
        <Field label="Time zone" hint="Optional. Briefs and schedules run in this time zone.">
          <Select
            class="px-2.5!"
            options={[{ value: "", label: "Not set" }, ...zoneChoices(props.draft.tz)]}
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
