import type { TailnetStatus } from "@marshal/protocol";

/** The tailnet port the phone app can reach without being told one: it tries this when none is typed. */
const APP_PORT = 47800;

/** The next thing for a person to do, in the daemon's own facts. */
export interface NextStep {
  title: string;
  detail: string;
  /** A command to run on this computer, which a screen shows with a Copy button. */
  command?: string;
  /** Another way to the same end, said in a sentence. */
  alternative?: string;
  /** An address to open, such as Tailscale's own sign-in page. */
  link?: { href: string; label: string };
}

/** How a phone reaches this computer, built only from what the daemon and Tailscale reported. */
export interface PhoneAccess {
  /** What a phone types: `name:port`. Null while no way for a phone to reach this computer is known. */
  address: string | null;
  /** Whether the daemon's own tailnet node or Tailscale on this computer carries the address. */
  via: "node" | "serve" | null;
  /** What was checked, so the screen says exactly how it knows. */
  checked: string;
  /** What to do when there is no address, or the address did not answer. */
  next: NextStep | null;
}

/** The command that has Tailscale hand a tailnet port to the daemon, as plain http for the phone app. */
export function serveCommand(tailnetPort: number, daemonPort: number): string {
  return `tailscale serve --bg --http=${tailnetPort} http://127.0.0.1:${daemonPort}`;
}

/** The tailnet port to suggest: the one the app needs no typing for, or the next one not in use. */
function freeTailnetPort(taken: readonly number[]): number {
  let port = APP_PORT;
  while (taken.includes(port)) port += 1;
  return port;
}

const HOST_STATE_ADVICE: Record<string, string> = {
  "not-running":
    "Tailscale is installed on this computer, but its app is not running. Open Tailscale, then check again.",
  stopped: "Tailscale is turned off on this computer. Turn it on, then check again.",
  "needs-login":
    "Tailscale is waiting for you to sign in on this computer. Sign in, then check again.",
  "needs-approval":
    "This computer is waiting to be approved in your Tailscale admin console. Approve it, then check again.",
  starting: "Tailscale is starting. Check again in a moment.",
};

const RESTART_WITH_TAILNET =
  "Or start Marshal with --tailnet (or MARSHAL_TAILNET=1), and it joins your tailnet itself, as a second device.";

/** The step for a Tailscale on this computer that is not running, or not there at all. */
function hostNotReady(status: TailnetStatus): NextStep {
  const { host } = status;
  if (!host.found) {
    return {
      title: "Tailscale is not on this computer",
      detail: "Install Tailscale and sign in on this computer, then check again.",
      alternative: RESTART_WITH_TAILNET,
    };
  }
  const detail =
    HOST_STATE_ADVICE[host.state] ??
    `Tailscale answered, but Marshal does not know what its state "${host.state}" means. Check the Tailscale app.`;
  return {
    title: "Tailscale is not ready on this computer",
    detail,
    alternative: RESTART_WITH_TAILNET,
  };
}

/** The step for a Tailscale that is running but does not hand the daemon a plain http port. */
function hostNeedsServe(status: TailnetStatus): NextStep {
  const { host } = status;
  const daemonPort = status.port || APP_PORT;
  const tailnetPort = freeTailnetPort(host.takenPorts);
  const portNote =
    tailnetPort === APP_PORT
      ? ""
      : ` Tailnet port ${APP_PORT} is already used by another Serve rule, so the phone has to type the port too: ${host.dnsName}:${tailnetPort}.`;
  if (host.secureServePort > 0) {
    return {
      title: "Tailscale Serve hands Marshal over https only",
      detail: `The phone app speaks plain http over the tailnet, so it cannot use port ${host.secureServePort}. Add a plain http rule on this computer, then check again.${portNote}`,
      command: serveCommand(tailnetPort, daemonPort),
      alternative: RESTART_WITH_TAILNET,
    };
  }
  return {
    title: "Tailscale is running here, but nothing hands a port to Marshal",
    detail: `This computer is ${host.dnsName} on your tailnet. Marshal listens on this computer only, so a phone cannot reach it yet. Run this on this computer, then check again.${portNote}`,
    command: serveCommand(tailnetPort, daemonPort),
    alternative: RESTART_WITH_TAILNET,
  };
}

/** The step when the daemon's own node is not online: signing in, or a failure to join. */
function nodeStep(status: TailnetStatus): NextStep | null {
  if (status.state === "signing-in") {
    return {
      title: "Sign in to Tailscale to finish",
      detail: "Marshal's own tailnet node is waiting for you to sign in.",
      link: status.loginUrl
        ? { href: status.loginUrl, label: "Open the Tailscale sign-in address" }
        : undefined,
    };
  }
  if (status.state === "error") {
    return {
      title: "Marshal could not join the tailnet",
      detail: status.error || "Tailscale did not say why.",
    };
  }
  return null;
}

/**
 * How a phone reaches this computer, from the tailnet status the daemon answered. The daemon's own
 * node wins when it is online. Otherwise Tailscale on this computer carries the address when Serve
 * hands a plain http port to the daemon. Otherwise there is no address, and `next` says what is
 * missing, with the real names and ports in it. Null while the status is not known.
 */
export function phoneAccess(status: TailnetStatus | null | undefined): PhoneAccess | null {
  if (!status) return null;
  const { host } = status;
  if (status.state === "online" && status.dnsName) {
    return {
      address: `${status.dnsName}:${status.port || APP_PORT}`,
      via: "node",
      checked: `Marshal joined your tailnet itself, as ${status.dnsName}, and the daemon listens on it.`,
      next: null,
    };
  }
  if (host.found && host.state === "running" && host.servePort > 0) {
    const address = `${host.dnsName}:${host.servePort}`;
    const rule = `Tailscale on this computer hands tailnet port ${host.servePort} to this daemon`;
    return host.reachable
      ? {
          address,
          via: "serve",
          checked: `${rule}, and this computer reached it there.`,
          next: null,
        }
      : {
          address,
          via: "serve",
          checked: `${rule}, but it did not answer when this computer asked.`,
          next: {
            title: "The address did not answer from this computer",
            detail:
              "The rule is there, but a request through it failed. Look at the rule with the command below.",
            command: "tailscale serve status",
          },
        };
  }
  const node = nodeStep(status);
  if (node)
    return {
      address: null,
      via: null,
      checked: "Marshal's own tailnet node is not online.",
      next: node,
    };
  const next =
    host.found && host.state === "running" ? hostNeedsServe(status) : hostNotReady(status);
  const checked = host.found
    ? `Asked Tailscale on this computer: it is ${host.state}.`
    : "Looked for Tailscale on this computer and did not find it.";
  return { address: null, via: null, checked, next };
}
