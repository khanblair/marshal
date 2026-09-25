import { readFile } from "node:fs/promises";
import { homedir } from "node:os";
import path from "node:path";
import type { Plugin } from "vite";

/** Where the dev server answers with the dev daemon's token. The page reads it in dev builds only. */
export const DEV_TOKEN_ROUTE = "/__marshal/dev-token";

const APP_FOLDER = "Marshal";
const DEV_SUFFIX = "-dev";
const TOKEN_FILE = "dev-token";
const OK = 200;
const FORBIDDEN = 403;
const NOT_FOUND = 404;
const METHOD_NOT_ALLOWED = 405;
const SERVER_ERROR = 500;

/** What the data folder depends on. Passed in, so a test can be any operating system. */
export interface MachineInfo {
  /** Node's `process.platform`: `darwin`, `win32`, or anything else, which follows the Linux rules. */
  platform: string;
  home: string;
  env: Readonly<Record<string, string | undefined>>;
}

/**
 * The dev daemon's data folder. It mirrors `DataDir` for the dev mode in
 * `daemon/internal/platform/paths.go`, and the `MARSHAL_DATA_DIR` override of
 * `daemon/internal/config`: a value there is used as it is. Keep the two in step.
 */
export function devDataDir({ platform, home, env }: MachineInfo): string | null {
  const override = env.MARSHAL_DATA_DIR;
  if (override) return override;
  if (!home) return null;
  const name = APP_FOLDER + DEV_SUFFIX;
  if (platform === "darwin") return path.posix.join(home, "Library", "Application Support", name);
  if (platform === "win32") {
    const base = env.APPDATA || path.win32.join(home, "AppData", "Roaming");
    return path.win32.join(base, name);
  }
  const base = env.XDG_DATA_HOME || path.posix.join(home, ".local", "share");
  return path.posix.join(base, name.toLowerCase());
}

/** The file where the dev daemon keeps its token, or null when the data folder cannot be known. */
export function devTokenPath(machine: MachineInfo): string | null {
  const dir = devDataDir(machine);
  if (dir === null) return null;
  return (machine.platform === "win32" ? path.win32 : path.posix).join(dir, TOKEN_FILE);
}

const IPV4_LOOPBACK = /^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/;
const MAPPED_PREFIX = "::ffff:";

/** True for an address of this machine: 127.0.0.0/8, ::1, and the same in IPv4-mapped form. */
export function isLoopback(address: string | undefined): boolean {
  if (!address) return false;
  const plain = address.startsWith(MAPPED_PREFIX) ? address.slice(MAPPED_PREFIX.length) : address;
  return plain === "::1" || IPV4_LOOPBACK.test(plain);
}

export interface RequestLike {
  method?: string | undefined;
  url?: string | undefined;
  socket: { remoteAddress?: string | undefined };
}

export interface ResponseLike {
  statusCode: number;
  setHeader(name: string, value: string): void;
  end(body?: string): void;
}

function reply(res: ResponseLike, status: number, body: unknown): void {
  res.statusCode = status;
  res.setHeader("Content-Type", "application/json");
  // The token changes when the dev data folder is reset, so no cache may keep an old one.
  res.setHeader("Cache-Control", "no-store");
  res.end(JSON.stringify(body));
}

type ReadText = (file: string) => Promise<string>;

function isMissingFile(error: unknown): boolean {
  return error instanceof Error && "code" in error && error.code === "ENOENT";
}

/** Reads the token file. Null means there is no token yet: the dev daemon has not started. */
async function readToken(file: string | null, read: ReadText): Promise<string | null> {
  if (file === null) return null;
  try {
    return (await read(file)).trim() || null;
  } catch (error) {
    if (isMissingFile(error)) return null;
    throw error;
  }
}

/**
 * The route's handler. The token is read from the file on every request, because Vite and the dev
 * daemon start at the same time and either may be first. It is served only to this machine: a
 * developer may start Vite with `--host`, and the token must never reach the network.
 */
export function createDevTokenHandler(options: { tokenPath: string | null; read?: ReadText }) {
  const read = options.read ?? ((file: string) => readFile(file, "utf8"));
  return async (req: RequestLike, res: ResponseLike, next: () => void): Promise<void> => {
    // Connect cuts the route off the address, so the route itself leaves "/" or nothing. It also
    // matches longer paths, and those are not ours.
    const rest = (req.url ?? "").split("?")[0];
    if (rest !== "" && rest !== "/") return next();
    if (!isLoopback(req.socket.remoteAddress)) {
      return reply(res, FORBIDDEN, { error: "The dev token is only given to this machine." });
    }
    if (req.method !== "GET" && req.method !== "HEAD") {
      res.setHeader("Allow", "GET, HEAD");
      return reply(res, METHOD_NOT_ALLOWED, { error: "Ask for the dev token with GET." });
    }
    try {
      const token = await readToken(options.tokenPath, read);
      if (token === null)
        return reply(res, NOT_FOUND, { error: "The dev daemon has not started yet." });
      reply(res, OK, { token });
    } catch {
      reply(res, SERVER_ERROR, { error: "The dev token could not be read." });
    }
  };
}

/** The machine this Vite runs on. */
function currentMachine(): MachineInfo {
  return { platform: process.platform, home: homedir(), env: process.env };
}

/**
 * Adds the dev token route to the dev server only. `apply: "serve"` keeps it out of `vite build`,
 * and it has no preview hook, so it exists under `vite` and nowhere else.
 */
export function devTokenPlugin(machine: MachineInfo = currentMachine()): Plugin {
  const handle = createDevTokenHandler({ tokenPath: devTokenPath(machine) });
  return {
    name: "marshal-dev-token",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use(DEV_TOKEN_ROUTE, (req, res, next) => {
        void handle(req, res, next);
      });
    },
  };
}
