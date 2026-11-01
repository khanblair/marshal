import { type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { bodyOf, keyOf, pathOf, refuse, STATUS } from "./fake-daemon-support";

/** What the device and folder routes need from the fake daemon: its clock and the token it holds. */
export interface DeviceRouter {
  /** The token the daemon holds as the right one, or null once it was revoked. */
  token: () => string | null;
  now: () => string;
}

/**
 * The paired devices (B9.1, B9.2, section S2b) and the tailnet status (B9.1): the four routes the
 * profile draws. The fake daemon answers with one phone, so a component test has a real list to
 * read; a revoke marks the row rather than dropping it, because the screen says a device was
 * removed instead of watching it vanish, and the status answers "off", which is what a daemon that
 * was never started with `--tailnet` truthfully is.
 */
/** The one pairing code the fake daemon makes and accepts. */
export const FAKE_PAIRING_CODE = "7QX-2LD";

const FAKE_DEVICES = [
  {
    id: "01H1234567890ABCDEFGHJKMNPQ",
    name: "Pixel 8",
    kind: "mobile",
    pairedAt: "2026-09-20T10:00:00.000Z",
    lastSeenAt: "2026-09-28T09:00:00.000Z",
    revoked: false,
  },
  {
    id: "01H1234567890ABCDEFGHJKMNPR",
    name: "iPad Air",
    kind: "mobile",
    pairedAt: "2026-09-22T12:00:00.000Z",
    lastSeenAt: null,
    revoked: false,
  },
];

export function answerDeviceRoute(router: DeviceRouter, request: FakeRequest): Response | null {
  const key = keyOf(request);
  if (key === "GET /v1/me/devices") {
    return jsonAnswer({ devices: FAKE_DEVICES, serverTime: router.now() });
  }
  if (key === "POST /v1/me/devices/pairing-code") {
    return jsonAnswer({
      code: FAKE_PAIRING_CODE,
      expiresAt: "2026-09-28T10:00:00.000Z",
      serverTime: router.now(),
    });
  }
  if (request.method === "DELETE" && pathOf(request.url).startsWith("/v1/me/devices/")) {
    return new Response(null, { status: 204 });
  }
  if (key === "GET /v1/tailnet") {
    return jsonAnswer({
      enabled: false,
      state: "off",
      hostname: "",
      dnsName: "",
      ips: [],
      port: 47800,
      identity: "",
      loginUrl: "",
      funnel: false,
      error: "",
      host: {
        found: false,
        state: "",
        dnsName: "",
        ips: [],
        account: "",
        tailnet: "",
        servePort: 0,
        secureServePort: 0,
        takenPorts: [],
        reachable: false,
        phones: [],
      },
      serverTime: router.now(),
    });
  }
  return null;
}

/**
 * The route that trades a pairing code for a token, which takes no token of its own. The one code
 * the fake makes is accepted and answers the daemon's own token, so a device that pairs is signed in;
 * any other is refused with the daemon's own 401, the same for a wrong, spent, or expired code.
 */
export function answerPairRoute(router: DeviceRouter, request: FakeRequest): Response {
  const body = bodyOf(request);
  if (body.code !== FAKE_PAIRING_CODE) {
    return refuse(
      STATUS.unauthorized,
      "unauthorized",
      "Sign in again. This device's token is missing or no longer valid.",
    );
  }
  const device = {
    ...FAKE_DEVICES[0],
    name: String(body.name ?? ""),
    kind: String(body.kind ?? ""),
  };
  return jsonAnswer({ token: router.token(), device, serverTime: router.now() });
}

/** The folders of a small pretend disk: the home folder, a code folder with one repository, and a notes folder. */
const FAKE_HOME = "/Users/ada";
const FAKE_DISK: Record<string, { name: string; repo?: boolean }[]> = {
  [FAKE_HOME]: [{ name: "code" }, { name: "notes" }],
  [`${FAKE_HOME}/code`]: [{ name: "marshal", repo: true }, { name: "scratch" }],
  [`${FAKE_HOME}/code/marshal`]: [{ name: "apps" }],
};

export function answerFolders(router: Pick<DeviceRouter, "now">, request: FakeRequest): Response {
  const asked = new URL(request.url, "http://fake-daemon").searchParams.get("path") ?? "";
  const path = asked === "" || asked === "~" ? FAKE_HOME : asked;
  const inside = FAKE_DISK[path];
  if (!inside && !Object.keys(FAKE_DISK).some((known) => known.startsWith(`${path}/`))) {
    return refuse(
      STATUS.notFound,
      "not_found",
      "Marshal cannot find that folder. It may have been removed.",
    );
  }
  const parent = path === "/" ? "" : path.slice(0, path.lastIndexOf("/")) || "/";
  return jsonAnswer({
    path,
    parent,
    home: FAKE_HOME,
    isGitRepo: path === `${FAKE_HOME}/code/marshal`,
    folders: (inside ?? []).map((entry) => ({
      name: entry.name,
      path: `${path}/${entry.name}`,
      isGitRepo: entry.repo === true,
    })),
    truncated: false,
    serverTime: router.now(),
  });
}
