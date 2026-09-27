import type {
  Integration as WireIntegration,
  IntegrationList,
  IntegrationStatus,
} from "@marshal/protocol";
import { type ProviderTest, toProviderTest } from "./providers";

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
  /** The last connection test's result, absent when the connection was never tested. */
  lastTest?: ProviderTest;
}

/** One connection as the daemon reports it, apart from the app's own words. */
function toState(entry: WireIntegration): IntegrationState {
  return {
    id: entry.id,
    st: entry.st,
    detail: entry.detail,
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
