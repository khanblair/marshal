import { RetryDoingNowPrefix } from "@marshal/protocol";

/** True when a card's "doing now" line says its agent is waiting for the network, such as "Waiting for network, retry 3 of 10". */
export function isNetworkWait(doing: string): boolean {
  return doing.startsWith(RetryDoingNowPrefix);
}
