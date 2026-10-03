import { Avatar, Button, Field, Input, Select, SettingsSection } from "@marshal/ui";
import { zoneChoices } from "~/data/time-zones";
import { M } from "~/mock";
import { GRID_MIN_220 } from "./auto-fit-grid";
import type { ProfileDraft } from "./use-profile-draft";

/**
 * The zones to offer: every one the browser knows, and the one the person already has when it is
 * not among them (the daemon takes any zone), with "Not set" first for a new daemon, which has none.
 */
function zoneOptions(current: string): readonly (string | { value: string; label: string })[] {
  const zones = zoneChoices(current);
  return current === "" ? [{ value: "", label: "Not set" }, ...zones] : zones;
}

/** Profile: name, email, time zone, and avatar. Paired devices and the tailnet are under Remote control. */
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
        <Avatar
          size={64}
          bordered
          aria-label="Your avatar"
          initials={props.profile.initials()}
          src={M.S.profile.avatar ?? undefined}
        />
        <div class="flex flex-wrap gap-2">
          <Button onClick={() => M.chooseAvatar()}>Upload image</Button>
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
            options={zoneOptions(fields().tz)}
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
    </SettingsSection>
  );
}
