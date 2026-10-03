import { Button, SettingsSection } from "@marshal/ui";
import { createResource, Show } from "solid-js";
import { M } from "~/mock";
import { phoneAccess } from "~/platform/phone-access";
import { DevicesList } from "./DevicesList";
import { PairDevice } from "./PairDevice";
import { PhoneAddress } from "./PhoneAddress";
import { TailnetFacts } from "./TailnetFacts";
import { TailnetPhones } from "./TailnetPhones";
import type { RemoteDraft } from "./use-remote-draft";

const SUBHEADING = "mt-2 mb-0 text-subtitle leading-5.5 font-semibold";

/**
 * Remote control: how a phone reaches this computer, and which devices may. The page asks the daemon
 * once, and the daemon asks Tailscale on this computer, so everything shown is what was found. Check
 * again asks once more, which is what to press after running a command from this page.
 */
export function RemoteSection(props: { draft: RemoteDraft }) {
  const [status, { refetch }] = createResource(async () => await M.tailnetStatus());
  const answer = () => status() ?? null;
  return (
    <SettingsSection
      title="Remote control"
      description="Control Marshal from your phone. The agents keep running on this computer."
      actions={
        <Button disabled={status.loading} onClick={() => void refetch()}>
          {status.loading ? "Checking…" : "Check again"}
        </Button>
      }
    >
      <Show when={phoneAccess(answer())}>{(access) => <PhoneAddress access={access()} />}</Show>
      <PairDevice draft={props.draft} status={answer()} />
      <h3 class={SUBHEADING}>Paired devices</h3>
      <DevicesList />
      <TailnetPhones status={answer()} />
      <TailnetFacts status={answer()} />
    </SettingsSection>
  );
}
