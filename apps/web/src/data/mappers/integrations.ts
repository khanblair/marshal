import type {
  GitHubConnect,
  GitHubInstallation,
  IntegrationList,
  IntegrationStatus,
  Integration as WireIntegration,
} from "@marshal/protocol";
import { type ProviderTest, toProviderTest } from "./providers";
import { toMillis } from "./time";

/**
 * One connection's last test. It is the provider test's shape exactly: a connection's test answers
 * what a provider's does (`docs/architecture.md` section 18), so one screen shows both the same way.
 */
export type IntegrationTest = ProviderTest;

/**
 * The daemon-owned half of one connection row (section S29a): the connection's own status, the one
 * sentence the daemon writes about it, and the last test's result. The words a person reads - the
 * name and the icon - are the app's, so they are not here: the store keeps its own row per
 * connection and a snapshot replaces only the fields the daemon owns, keyed by the connection's id.
 *
 * A connection's test answers the same shape a provider's does (`docs/architecture.md` section 18),
 * so the result is the provider one: one screen shows both kinds the same way.
 */
export interface IntegrationState {
  /** The connection's own id, such as "github": how a snapshot's entry finds the store's row. */
  id: string;
  st: IntegrationStatus;
  detail: string;
  /** Where a chat connection sends notices (a channel id, a chat id, a topic), which is not a secret. */
  target?: string;
  /** The last connection test's result, absent when the connection was never tested. */
  lastTest?: ProviderTest;
}

/** One connection as the daemon reports it, apart from the app's own words. */
function toState(entry: WireIntegration): IntegrationState {
  return {
    id: entry.id,
    st: entry.st,
    detail: entry.detail,
    ...(entry.target ? { target: entry.target } : {}),
    ...(entry.lastTest ? { lastTest: toProviderTest(entry.lastTest) } : {}),
  };
}

/**
 * The whole list answer, in the order the daemon sent it. A change route answers the same shape, so
 * the same function applies a read, a save, and a remove: every connection route answers the list.
 */
export function toIntegrationStates(answer: IntegrationList): IntegrationState[] {
  return answer.integrations.map(toState);
}

/** Where the GitHub sign-in is; one settings panel is drawn for each. */
export type GitHubConnectState =
  | "idle"
  | "pending"
  | "needs_install"
  | "connected"
  | "denied"
  | "expired"
  | "failed";

/** One account the Marshal GitHub App is installed on. */
export interface GitHubAccount {
  account: string;
  kind: "user" | "organization";
  allRepositories: boolean;
}

/**
 * The GitHub connection as the sign-in dialog reads it (section S29a). Every field the daemon
 * leaves out for a state is absent here too, so a screen asks for what its state has.
 */
export interface GitHubConnection {
  state: GitHubConnectState;
  /** How GitHub is connected once it is: by a sign-in or by a pasted token. */
  mode?: "oauth" | "token";
  /** The code the person types on GitHub, while the sign-in is pending. */
  userCode?: string;
  verificationUri?: string;
  /** When the pending code stops working, in ms since 1970. */
  expiresAt?: number;
  /** The GitHub user Marshal is signed in as. */
  login?: string;
  /** The page that installs the Marshal GitHub App on an account. */
  installUrl?: string;
  installations: GitHubAccount[];
  /** One plain sentence the daemon wrote for a state that needs explaining. */
  message?: string;
}

const CONNECT_STATES: readonly GitHubConnectState[] = [
  "idle",
  "pending",
  "needs_install",
  "connected",
  "denied",
  "expired",
  "failed",
];

/** What a person reads when the daemon answers a state this app has no panel for. */
const UNKNOWN_STATE_MESSAGE = "Marshal got an answer from GitHub it does not know how to show.";

/** The wire's states are one string type, so an unknown one is narrowed to a failure rather than guessed. */
function toConnectState(state: string): GitHubConnectState {
  return CONNECT_STATES.find((known) => known === state) ?? "failed";
}

function toAccount(entry: GitHubInstallation): GitHubAccount {
  return {
    account: entry.account,
    kind: entry.kind === "organization" ? "organization" : "user",
    allRepositories: entry.allRepositories,
  };
}

/** The daemon's GitHub sign-in answer, with its timestamp as ms and its two loose strings narrowed. */
export function toGitHubConnection(answer: GitHubConnect): GitHubConnection {
  const state = toConnectState(answer.state);
  const message = answer.message || (state === answer.state ? "" : UNKNOWN_STATE_MESSAGE);
  return {
    state,
    ...(answer.mode === "oauth" || answer.mode === "token" ? { mode: answer.mode } : {}),
    ...(answer.userCode ? { userCode: answer.userCode } : {}),
    ...(answer.verificationUri ? { verificationUri: answer.verificationUri } : {}),
    ...(answer.expiresAt ? { expiresAt: toMillis(answer.expiresAt) } : {}),
    ...(answer.login ? { login: answer.login } : {}),
    ...(answer.installUrl ? { installUrl: answer.installUrl } : {}),
    installations: answer.installations.map(toAccount),
    ...(message ? { message } : {}),
  };
}
