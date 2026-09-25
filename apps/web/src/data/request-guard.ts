/** Why a request was stopped by the client: too slow, or cancelled by the caller. */
type StopCause = "timeout" | "aborted";

export interface RequestGuard {
  /** Give this to `fetch`. It aborts when the time is up or the caller cancels. */
  signal: AbortSignal;
  /** Which of the two stopped the request, or null when neither did. */
  cause(): StopCause | null;
  /** Stops the timer and the listener. Call it when the request is over. */
  release(): void;
}

/**
 * Joins a time limit and the caller's own `AbortSignal` into one signal. It builds both from a
 * plain `AbortController` and `setTimeout` (and not `AbortSignal.timeout` or `AbortSignal.any`),
 * so fake timers control it in tests and it works in every browser the app supports.
 */
export function guardRequest(caller: AbortSignal | undefined, timeoutMs: number): RequestGuard {
  const controller = new AbortController();
  let cause: StopCause | null = null;
  const stop = (why: StopCause) => {
    if (cause !== null) return;
    cause = why;
    controller.abort();
  };
  const timer = setTimeout(() => stop("timeout"), timeoutMs);
  const onCallerAbort = () => stop("aborted");
  if (caller?.aborted) onCallerAbort();
  else caller?.addEventListener("abort", onCallerAbort, { once: true });
  return {
    signal: controller.signal,
    cause: () => cause,
    release() {
      clearTimeout(timer);
      caller?.removeEventListener("abort", onCallerAbort);
    },
  };
}
