import { Avatar, Button, Field, Input, Select, SettingsSection } from "@marshal/ui";
import { createResource, Show } from "solid-js";
import { zoneChoices } from "~/data/time-zones";
import { M } from "~/mock";
import { GRID_MIN_220 } from "./auto-fit-grid";
import { DevicesList } from "./DevicesList";
import type { ProfileDraft } from "./use-profile-draft";

/**
 * The zones to offer: every one the browser knows, and the one the person already has when it is
 * not among them (the daemon takes any zone), with "Not set" first for a new daemon, which has none.
 */
function zoneOptions(current: string): readonly (string | { value: string; label: string })[] {
  const zones = zoneChoices(current);
  return current === "" ? [{ value: "", label: "Not set" }, ...zones] : zones;
}

const SUBHEADING = "mt-2 mb-0 text-subtitle leading-5.5 font-semibold";

/**
 * How reachable this daemon is, by the node's own four words (B9.1, build-plan 9.9). They are
 * deliberately plain: "signing in" is the state a person has to act on, and it says so by naming
 * the address the node wants them to open rather than by showing a spinner.
 */
const REACHABLE: Record<string, string> = {
  off: "This machine only",
  "signing-in": "Waiting for sign-in",
  online: "On your tailnet",
  error: "Could not join",
};

function TailnetIdentity() {
  // Asked for once when the section draws. The daemon answers "off" at once when it was never
  // started to join a tailnet, so this never waits on a node that does not exist.
  const [status] = createResource(async () => await M.tailnetStatus());
  const node = () => status() ?? null;
  return (
    <>
      <h3 class={SUBHEADING}>Tailnet identity</h3>
      <div class="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-body">
        <span class="text-secondary">Account</span>
        <span>{node()?.identity || M.S.profile.tailnet || "Not signed in"}</span>
        <span class="text-secondary">This machine</span>
        <Show when={node()?.dnsName || M.S.profile.node} fallback={<span>Not on a tailnet</span>}>
          {(name) => <code class="font-mono text-small">{name()}</code>}
        </Show>
        <span class="text-secondary">Reachable</span>
        <span>{REACHABLE[node()?.state ?? "off"] ?? REACHABLE.off}</span>
        {/* Funnel says what was asked for, not what Tailscale has allowed: the tailnet itself has to
            allow it too, which is done in the Tailscale admin console, not here. */}
        <Show when={node()?.funnel}>
          <span class="text-secondary">Public webhooks</span>
          <span>Exposed through Tailscale Funnel - only /hooks/*, each request still signed.</span>
        </Show>
        <Show when={node()?.state === "error" && node()?.error}>
          <span class="text-secondary">Why</span>
          <span class="text-status-danger-text">{node()?.error}</span>
        </Show>
        <Show when={node()?.loginUrl}>
          <span class="text-secondary">Sign in</span>
          <a href={node()?.loginUrl} target="_blank" rel="noreferrer" class="underline">
            Open the Tailscale sign-in address
          </a>
        </Show>
      </div>
      {/* Only the daemon's own "off" says this: a mock has no daemon to start differently. */}
      <Show when={node()?.state === "off"}>
        <p class="m-0 text-small leading-4.5 text-secondary max-w-prose">
          Your phone cannot reach this computer yet. Start Marshal with{" "}
          <code class="font-mono">--tailnet</code> (or set{" "}
          <code class="font-mono">MARSHAL_TAILNET=1</code>), then sign in to Tailscale here. The
          phone needs the Tailscale app, signed in to the same account.
        </p>
      </Show>
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
      <h3 class={SUBHEADING}>Paired devices</h3>
      <DevicesList profile={props.profile} />
      <TailnetIdentity />
    </SettingsSection>
  );
}
