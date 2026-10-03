import type { TailnetStatus } from "@marshal/protocol";
import { Show } from "solid-js";
import { M } from "~/mock";

const SUBHEADING = "mt-2 mb-0 text-subtitle leading-5.5 font-semibold";

/** The daemon's own tailnet node, by its four words (B9.1, build-plan 9.9). */
const NODE: Record<string, string> = {
  off: "Not started",
  "signing-in": "Waiting for sign-in",
  online: "On your tailnet",
  error: "Could not join",
};

/** Tailscale on this computer, by the words its own state maps to. */
const TAILSCALE: Record<string, string> = {
  running: "Running",
  starting: "Starting",
  "needs-login": "Waiting for sign-in",
  "needs-approval": "Waiting for approval",
  stopped: "Turned off",
  "not-running": "Its app is not running",
  unknown: "State unknown",
};

/**
 * What this computer is on the tailnet, as Tailscale and Marshal's own node report it. The prototype
 * has no daemon to ask, and shows its own account and machine in these rows.
 */
export function TailnetFacts(props: { status: TailnetStatus | null | undefined }) {
  const host = () => props.status?.host;
  const known = () => Boolean(props.status);
  const online = () => props.status?.state === "online";
  const account = () =>
    (online() && props.status?.identity) ||
    host()?.account ||
    M.S.profile.tailnet ||
    "Not signed in";
  const machine = () =>
    (online() && props.status?.dnsName) || host()?.dnsName || M.S.profile.node || "";
  return (
    <>
      <h3 class={SUBHEADING}>This computer</h3>
      <div class="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-body">
        <span class="text-secondary">Account</span>
        <span>{account()}</span>
        <span class="text-secondary">This machine</span>
        <Show when={machine()} fallback={<span>Not on a tailnet</span>}>
          {(name) => <code class="font-mono text-small">{name()}</code>}
        </Show>
        <Show when={known()}>
          <span class="text-secondary">Tailscale here</span>
          <span>
            {host()?.found
              ? (TAILSCALE[host()?.state ?? ""] ?? TAILSCALE.unknown)
              : "Not installed"}
          </span>
          <span class="text-secondary">Marshal's own node</span>
          <span>{NODE[props.status?.state ?? "off"] ?? NODE.off}</span>
        </Show>
        {/* Funnel says what was asked for, not what Tailscale has allowed: the tailnet itself has to
            allow it too, which is done in the Tailscale admin console, not here. */}
        <Show when={props.status?.funnel}>
          <span class="text-secondary">Public webhooks</span>
          <span>Exposed through Tailscale Funnel - only /hooks/*, each request still signed.</span>
        </Show>
        <Show when={props.status?.state === "error" && props.status?.error}>
          <span class="text-secondary">Why</span>
          <span class="text-status-danger-text">{props.status?.error}</span>
        </Show>
      </div>
    </>
  );
}
