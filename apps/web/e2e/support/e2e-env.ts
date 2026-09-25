import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

/**
 * Where the end-to-end run keeps its own daemon. Nothing here is the developer's real data folder
 * or the default dev port (47801): the run has a throwaway folder and a port of its own.
 */
export const VITE_PORT = 5299;
export const DAEMON_PORT = 47_811;

const webRoot = resolve(fileURLToPath(new URL("../..", import.meta.url)));

/** The daemon's data folder for the run, made new each time by `scripts/e2e-daemon.mjs`. */
export const DATA_DIR = resolve(webRoot, "test-results", "daemon-data");
/** The process that owns the daemon writes its id here, so a spec can stop and start the daemon. */
export const RUNNER_PID_FILE = resolve(webRoot, "test-results", "e2e-daemon.pid");
export const DAEMON_ORIGIN = `http://127.0.0.1:${DAEMON_PORT}`;
