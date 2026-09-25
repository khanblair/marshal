import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it, vi } from "vitest";
import {
  createDevTokenHandler,
  DEV_TOKEN_ROUTE,
  devDataDir,
  devTokenPath,
  devTokenPlugin,
  isLoopback,
  type MachineInfo,
  type RequestLike,
  type ResponseLike,
} from "../../vite/dev-token";
import { REPO_ROOT } from "./testing/golden";

const machine = (
  platform: string,
  home: string,
  env: Record<string, string> = {},
): MachineInfo => ({
  platform,
  home,
  env,
});

describe("the dev data folder, mirrored from daemon/internal/platform/paths.go", () => {
  it.each([
    [
      "macOS",
      machine("darwin", "/Users/sam"),
      "/Users/sam/Library/Application Support/Marshal-dev",
    ],
    [
      "Linux with no XDG folder",
      machine("linux", "/home/sam"),
      "/home/sam/.local/share/marshal-dev",
    ],
    [
      "Linux with an XDG folder",
      machine("linux", "/home/sam", { XDG_DATA_HOME: "/data/x" }),
      "/data/x/marshal-dev",
    ],
    [
      "Linux with an empty XDG setting",
      machine("linux", "/home/sam", { XDG_DATA_HOME: "" }),
      "/home/sam/.local/share/marshal-dev",
    ],
    [
      "an operating system it does not know",
      machine("freebsd", "/home/sam"),
      "/home/sam/.local/share/marshal-dev",
    ],
    [
      "Windows with APPDATA",
      machine("win32", "C:\\Users\\sam", { APPDATA: "C:\\Users\\sam\\AppData\\Roaming" }),
      "C:\\Users\\sam\\AppData\\Roaming\\Marshal-dev",
    ],
    [
      "Windows without APPDATA",
      machine("win32", "C:\\Users\\sam"),
      "C:\\Users\\sam\\AppData\\Roaming\\Marshal-dev",
    ],
  ])("is right for %s", (_name, info, expected) => {
    expect(devDataDir(info)).toBe(expected);
  });

  it.each(["darwin", "linux", "win32"])("uses MARSHAL_DATA_DIR as it is on %s", (platform) => {
    const info = machine(platform, "/home/sam", {
      MARSHAL_DATA_DIR: "somewhere/else",
      APPDATA: "x",
      XDG_DATA_HOME: "y",
    });
    expect(devDataDir(info)).toBe("somewhere/else");
  });

  it("does not need the home folder when the data folder is set, and has no folder without either", () => {
    expect(devDataDir(machine("linux", "", { MARSHAL_DATA_DIR: "/data" }))).toBe("/data");
    expect(devDataDir(machine("linux", ""))).toBeNull();
    expect(devTokenPath(machine("linux", ""))).toBeNull();
  });

  it.each([
    [
      "macOS",
      machine("darwin", "/Users/sam"),
      "/Users/sam/Library/Application Support/Marshal-dev/dev-token",
    ],
    ["Linux", machine("linux", "/home/sam"), "/home/sam/.local/share/marshal-dev/dev-token"],
    [
      "Windows",
      machine("win32", "C:\\Users\\sam"),
      "C:\\Users\\sam\\AppData\\Roaming\\Marshal-dev\\dev-token",
    ],
    [
      "Linux with MARSHAL_DATA_DIR",
      machine("linux", "/h", { MARSHAL_DATA_DIR: "/tmp/x" }),
      "/tmp/x/dev-token",
    ],
    [
      "Windows with MARSHAL_DATA_DIR",
      machine("win32", "C:\\h", { MARSHAL_DATA_DIR: "D:\\dev-data" }),
      "D:\\dev-data\\dev-token",
    ],
  ])("puts the token file in the folder on %s", (_name, info, expected) => {
    expect(devTokenPath(info)).toBe(expected);
  });

  it("still agrees with the Go source it copies", () => {
    const go = (file: string) =>
      readFileSync(resolve(REPO_ROOT, "daemon", "internal", file), "utf8");
    const paths = go("platform/paths.go");
    expect(paths).toContain('appFolder = "Marshal"');
    expect(paths).toContain('devSuffix = "-dev"');
    expect(paths).toContain('"Library", "Application Support"');
    expect(paths).toContain('env.Getenv("APPDATA")');
    expect(paths).toContain('"AppData", "Roaming"');
    expect(paths).toContain('env.Getenv("XDG_DATA_HOME")');
    expect(paths).toContain('".local", "share"');
    expect(paths).toContain("strings.ToLower(name)");
    expect(go("config/defaults.go")).toContain('envDataDir  = "MARSHAL_DATA_DIR"');
    expect(go("platform/tokens.go")).toContain('DevTokenFile = "dev-token"');
  });
});

describe("isLoopback", () => {
  it.each(["127.0.0.1", "127.0.0.2", "127.255.255.255", "::1", "::ffff:127.0.0.1"])(
    "accepts %s",
    (address) => {
      expect(isLoopback(address)).toBe(true);
    },
  );

  it.each([
    undefined,
    "",
    "192.168.1.20",
    "10.0.0.5",
    "128.0.0.1",
    "1270.0.0.1",
    "127.0.0.1.evil.example",
    "::ffff:10.0.0.5",
    "fe80::1",
    "::2",
    "100.101.102.103",
  ])("refuses %s", (address) => {
    expect(isLoopback(address)).toBe(false);
  });
});

interface Sent {
  status: number;
  headers: Record<string, string>;
  body: string;
}

function respond() {
  const sent: Sent = { status: 0, headers: {}, body: "" };
  const res: ResponseLike = {
    statusCode: 0,
    setHeader: (name, value) => {
      sent.headers[name.toLowerCase()] = value;
    },
    end: (body = "") => {
      sent.status = res.statusCode;
      sent.body = body;
    },
  };
  return { res, sent };
}

async function ask(
  handler: ReturnType<typeof createDevTokenHandler>,
  request: Partial<RequestLike> & { address?: string | undefined } = {},
) {
  const { res, sent } = respond();
  const next = vi.fn();
  const req: RequestLike = {
    method: request.method ?? "GET",
    url: request.url ?? "",
    socket: { remoteAddress: "address" in request ? request.address : "127.0.0.1" },
  };
  await handler(req, res, next);
  return { sent, next };
}

describe("the dev token route", () => {
  const TOKEN = "dev-token-not-real";
  const file = "/dev/data/dev-token";

  it("answers with the token in the file, trimmed, and lets no cache keep it", async () => {
    const read = vi.fn(async () => `${TOKEN}\n`);
    const { sent } = await ask(createDevTokenHandler({ tokenPath: file, read }));
    expect(sent.status).toBe(200);
    expect(JSON.parse(sent.body)).toEqual({ token: TOKEN });
    expect(sent.headers["cache-control"]).toBe("no-store");
    expect(sent.headers["content-type"]).toBe("application/json");
    expect(read).toHaveBeenCalledWith(file);
  });

  it("reads the file on every request, so a daemon that starts later or is reset is followed", async () => {
    let content: string | null = null;
    const read = vi.fn(async () => {
      if (content === null) throw Object.assign(new Error("no such file"), { code: "ENOENT" });
      return content;
    });
    const handler = createDevTokenHandler({ tokenPath: file, read });
    expect((await ask(handler)).sent.status).toBe(404);
    content = "first\n";
    expect(JSON.parse((await ask(handler)).sent.body)).toEqual({ token: "first" });
    content = "second\n";
    expect(JSON.parse((await ask(handler)).sent.body)).toEqual({ token: "second" });
    expect(read).toHaveBeenCalledTimes(3);
  });

  it("answers 404 while the file is missing or empty, or when the folder is unknown", async () => {
    const missing = createDevTokenHandler({
      tokenPath: file,
      read: async () =>
        Promise.reject(Object.assign(new Error("no such file"), { code: "ENOENT" })),
    });
    const empty = createDevTokenHandler({ tokenPath: file, read: async () => " \n" });
    const unknown = createDevTokenHandler({ tokenPath: null });
    for (const handler of [missing, empty, unknown]) {
      const { sent } = await ask(handler);
      expect(sent.status).toBe(404);
      expect(sent.body).not.toContain('token":');
    }
  });

  it("answers 500 with a plain sentence when the file cannot be read, and never names the file", async () => {
    const handler = createDevTokenHandler({
      tokenPath: file,
      read: async () =>
        Promise.reject(Object.assign(new Error(`EACCES ${file}`), { code: "EACCES" })),
    });
    const { sent } = await ask(handler);
    expect(sent.status).toBe(500);
    expect(sent.body).not.toContain(file);
  });

  it.each(["192.168.1.20", "10.0.0.5", "::ffff:10.0.0.5", "100.101.102.103", undefined])(
    "answers 403 to a request from %s and does not even read the file",
    async (address) => {
      const read = vi.fn(async () => TOKEN);
      const { sent } = await ask(createDevTokenHandler({ tokenPath: file, read }), { address });
      expect(sent.status).toBe(403);
      expect(sent.body).not.toContain(TOKEN);
      expect(read).not.toHaveBeenCalled();
    },
  );

  it.each(["127.0.0.1", "::1", "::ffff:127.0.0.1"])("serves a request from %s", async (address) => {
    const { sent } = await ask(
      createDevTokenHandler({ tokenPath: file, read: async () => TOKEN }),
      { address },
    );
    expect(sent.status).toBe(200);
  });

  it("serves a HEAD request and refuses other methods", async () => {
    const handler = createDevTokenHandler({ tokenPath: file, read: async () => TOKEN });
    expect((await ask(handler, { method: "HEAD" })).sent.status).toBe(200);
    const post = (await ask(handler, { method: "POST" })).sent;
    expect(post.status).toBe(405);
    expect(post.headers.allow).toBe("GET, HEAD");
  });

  it("leaves other paths under the route to the next handler", async () => {
    const handler = createDevTokenHandler({ tokenPath: file, read: async () => TOKEN });
    const { sent, next } = await ask(handler, { url: "/other" });
    expect(next).toHaveBeenCalledTimes(1);
    expect(sent.status).toBe(0);
    const query = await ask(handler, { url: "/?x=1" });
    expect(query.sent.status).toBe(200);
    expect((await ask(handler, { url: "/" })).sent.status).toBe(200);
  });
});

describe("the plugin", () => {
  it("serves only under the dev server: it applies to serve, and has no preview hook", () => {
    const plugin = devTokenPlugin(machine("linux", "/home/sam"));
    expect(plugin.name).toBe("marshal-dev-token");
    expect(plugin.apply).toBe("serve");
    expect(plugin).not.toHaveProperty("configurePreviewServer");
  });

  it("adds the route to the dev server's middlewares", async () => {
    const plugin = devTokenPlugin(machine("linux", "/nonexistent-home-for-test"));
    const use = vi.fn();
    const configure = plugin.configureServer;
    if (typeof configure !== "function") throw new Error("the plugin has no configureServer");
    configure.call({} as never, { middlewares: { use } } as never);
    expect(use).toHaveBeenCalledTimes(1);
    expect(use.mock.calls[0]?.[0]).toBe(DEV_TOKEN_ROUTE);
    expect(DEV_TOKEN_ROUTE).toBe("/__marshal/dev-token");
    // Through the middleware, for a request from the network: refused.
    const { res, sent } = respond();
    const middleware = use.mock.calls[0]?.[1] as (
      a: RequestLike,
      b: ResponseLike,
      c: () => void,
    ) => void;
    middleware(
      { method: "GET", url: "", socket: { remoteAddress: "192.168.1.9" } },
      res,
      () => undefined,
    );
    await vi.waitFor(() => expect(sent.status).toBe(403));
  });

  it("is wired into the Vite config", () => {
    const config = readFileSync(resolve(REPO_ROOT, "apps", "web", "vite.config.ts"), "utf8");
    // The extension is required: Vite's native config loader cannot resolve a relative import
    // without one, and it warns about it on every start.
    expect(config).toContain('import { devTokenPlugin } from "./vite/dev-token.ts"');
    expect(config).toContain("devTokenPlugin()");
  });
});
