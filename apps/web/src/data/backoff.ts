export interface BackoffPolicy {
  /** The wait before the first retry, in ms. It doubles with each failed attempt. */
  baseMs: number;
  /** The longest wait before jitter, in ms. */
  capMs: number;
  /** The share of the wait that may be added or taken away at random: 0.2 is plus or minus 20 percent. */
  jitter: number;
}

/**
 * How long to wait before retrying after `failed` failed attempts (0 for the first retry):
 * `min(cap, base * 2^failed)`, moved by up to the jitter either way. `random` returns a number from
 * 0 up to 1, and 0.5 gives no jitter. It is a parameter so a test can pin the wait.
 */
export function backoffDelay(
  failed: number,
  policy: BackoffPolicy,
  random: () => number = Math.random,
): number {
  const wait = Math.min(policy.capMs, policy.baseMs * 2 ** failed);
  const spread = (random() * 2 - 1) * policy.jitter;
  return Math.round(wait * (1 + spread));
}
