import { type Accessor, batch, createSignal } from "solid-js";
import type { ApiError } from "./api-error";
import {
  type ConnectionMachine,
  type ConnectionState,
  createConnectionMachine,
  type MachineDeps,
} from "./connection-machine";

/** The connection as the screens read it: three signals, and the machine's calls. */
export interface Connection extends ConnectionMachine {
  state: Accessor<ConnectionState>;
  lastError: Accessor<ApiError | null>;
  /** When the next check happens, in ms on the local clock. Use it for "trying again in 4 s". */
  retryAt: Accessor<number | null>;
}

/** The state machine of the connection with a thin layer of Solid signals on top. */
export function createConnection(deps: Omit<MachineDeps, "onChange">): Connection {
  const [state, setState] = createSignal<ConnectionState>("starting");
  const [lastError, setLastError] = createSignal<ApiError | null>(null);
  const [retryAt, setRetryAt] = createSignal<number | null>(null);
  const machine = createConnectionMachine({
    ...deps,
    onChange: (next) =>
      batch(() => {
        setState(next.state);
        setLastError(next.lastError);
        setRetryAt(next.retryAt);
      }),
  });
  return { ...machine, state, lastError, retryAt };
}
