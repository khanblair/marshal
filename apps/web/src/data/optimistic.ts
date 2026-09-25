import { ApiError } from "./api-error";

interface OptimisticOptions<T> {
  /** Names the change. A second call with the same key while the first is running is refused. */
  key?: string;
  /** Changes the screen state at once, before the daemon has answered. */
  apply: () => void;
  /** Asks the daemon. Its result is the result of the whole call. */
  request: () => Promise<T>;
  /** Puts the screen state back. It runs once, and only when the request failed. */
  rollback: () => void;
  /** Words for a failure, when the message of the error is not the right one. Undefined means use it. */
  describeError?: (error: unknown) => string | undefined;
}

/** Runs a change on the screen first, asks the daemon, and puts the change back if it says no. */
export type Optimistic = <T>(options: OptimisticOptions<T>) => Promise<T>;

const GENERIC_MESSAGE = "Marshal could not save that change. Try again.";
const BUSY_MESSAGE = "That change is still being saved. Wait a moment and try again.";

/** A second change with the same key was refused while the first is still running. Nothing was changed or shown. */
export class ChangeInFlightError extends Error {
  constructor() {
    super(BUSY_MESSAGE);
    this.name = "ChangeInFlightError";
  }
}

function messageFor(error: unknown, describe: OptimisticOptions<unknown>["describeError"]): string {
  const described = describe?.(error);
  if (described) return described;
  return error instanceof ApiError ? error.message : GENERIC_MESSAGE;
}

/**
 * The daemon owns the state, so a change on the screen is only a guess until it answers
 * (ui-rules.md section 1, rule 3). On a failure it rolls back once and shows one plain sentence
 * through `toast`: never a stack and never a code. It then throws the same error, so the caller
 * can also react. The keys in flight belong to this instance, not to the module.
 */
export function createOptimistic(toast: (message: string) => void): Optimistic {
  const running = new Set<string>();
  return async (options) => {
    const { key } = options;
    if (key !== undefined) {
      if (running.has(key)) throw new ChangeInFlightError();
      running.add(key);
    }
    try {
      options.apply();
      return await options.request();
    } catch (error) {
      try {
        options.rollback();
      } finally {
        toast(messageFor(error, options.describeError));
      }
      throw error;
    } finally {
      if (key !== undefined) running.delete(key);
    }
  };
}
