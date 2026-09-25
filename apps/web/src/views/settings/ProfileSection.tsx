import { Avatar, Button, Field, Input, Select, SettingsSection } from "@marshal/ui";
import { M } from "~/mock";
import { GRID_MIN_220 } from "./auto-fit-grid";
import { DevicesList } from "./DevicesList";
import type { ProfileDraft } from "./use-profile-draft";

const TIME_ZONES = [
  "Europe/London",
  "Europe/Lisbon",
  "Africa/Lagos",
  "America/New_York",
  "America/Los_Angeles",
  "Asia/Singapore",
];

const SUBHEADING = "mt-2 mb-0 text-subtitle leading-5.5 font-semibold";

function TailnetIdentity() {
  return (
    <>
      <h3 class={SUBHEADING}>Tailnet identity</h3>
      <div class="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-body">
        <span class="text-secondary">Account</span>
        <span>{M.S.profile.tailnet}</span>
        <span class="text-secondary">This machine</span>
        <code class="font-mono text-small">{M.S.profile.node}</code>
      </div>
    </>
  );
}

/** Profile: name, email, and time zone, the paired devices, and the tailnet identity. */
export function ProfileSection(props: { profile: ProfileDraft }) {
  const fields = () => props.profile.fields();
  return (
    <SettingsSection
      title="Profile"
      onSubmit={(event) => {
        event.preventDefault();
        props.profile.save();
      }}
    >
      <div class="flex flex-wrap items-center gap-4">
        <Avatar size={64} bordered aria-label="Your avatar" initials={props.profile.initials()} />
        <div class="flex flex-wrap gap-2">
          <Button onClick={() => M.toast("Choose an image to use as your avatar")}>
            Upload image
          </Button>
          <span class="self-center text-small text-secondary">
            Without an image, Marshal shows your initials.
          </span>
        </div>
      </div>
      <div class={GRID_MIN_220}>
        <Field
          label="Name"
          error={
            fields().name.trim() ? undefined : "Enter a name. It shows on cards you comment on."
          }
        >
          <Input
            value={fields().name}
            onInput={(e) => props.profile.edit("name", e.currentTarget.value)}
          />
        </Field>
        <Field label="Email" hint="Used only to send briefs by email.">
          <Input
            type="email"
            value={fields().email}
            placeholder="Optional"
            onInput={(e) => props.profile.edit("email", e.currentTarget.value)}
          />
        </Field>
        <Field label="Time zone" hint="Briefs and schedules run in this time zone.">
          <Select
            class="px-2.5!"
            options={TIME_ZONES}
            value={fields().tz}
            onChange={(e) => props.profile.edit("tz", e.currentTarget.value)}
          />
        </Field>
      </div>
      <div>
        <Button variant="primary" type="submit" disabled={props.profile.cannotSave()}>
          Save profile
        </Button>
      </div>
      <h3 class={SUBHEADING}>Paired devices</h3>
      <DevicesList profile={props.profile} />
      <TailnetIdentity />
    </SettingsSection>
  );
}
