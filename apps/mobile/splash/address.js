// The pure parts of the first screen: what a person typed or scanned, turned into the daemon's
// address. Kept apart from the page so they can be tested without a browser.

export const DEFAULT_PORT = 47800;
const HOST = /^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$/i;
const MAX_PORT = 65535;

/** Turns what was typed into `host:port`, or says why it cannot be one. */
export function normalizeAddress(text) {
  let rest = String(text ?? "")
    .trim()
    .replace(/^https?:\/\//i, "");
  rest = rest.split(/[/?#]/)[0] ?? "";
  if (!rest) return { ok: false, reason: "Type the address of your computer." };
  const [host, port, ...extra] = rest.split(":");
  if (extra.length > 0 || !HOST.test(host ?? "")) {
    return {
      ok: false,
      reason: "That does not look like an address. Use a name like marshal-laptop.tail1234.ts.net.",
    };
  }
  const number = port === undefined || port === "" ? DEFAULT_PORT : Number(port);
  if (!Number.isInteger(number) || number < 1 || number > MAX_PORT) {
    return { ok: false, reason: "The port must be a number from 1 to 65535." };
  }
  return { ok: true, address: `${host.toLowerCase()}:${number}` };
}

/**
 * Reads the QR code the computer shows, `marshal://pair?host=<address>&code=<code>`. It answers the
 * address and the code, or null for any other code, so a photo of something else never navigates.
 */
export function parseScan(text) {
  let url;
  try {
    url = new URL(String(text ?? "").trim());
  } catch {
    return null;
  }
  if (url.protocol !== "marshal:" || url.hostname !== "pair") return null;
  const address = normalizeAddress(url.searchParams.get("host") ?? "");
  const code = (url.searchParams.get("code") ?? "").trim();
  return address.ok && code ? { address: address.address, code } : null;
}

/** The page the daemon serves, with the pairing code when there is one to hand over. */
export function daemonUrl(address, code) {
  return `http://${address}/${code ? `?pair=${encodeURIComponent(code)}` : ""}`;
}

/**
 * What a camera permission answer says. The plugin answers `{ camera: "granted" }`, not the bare word,
 * so comparing the whole answer to "granted" is never true; a bare word is read as it is.
 */
export function permissionState(answer) {
  if (typeof answer === "string") return answer;
  if (answer && typeof answer === "object") return String(answer.camera ?? "");
  return "";
}

/** The first Marshal pairing link among the links the phone handed over, or null. */
export function pairingFrom(urls) {
  for (const url of Array.isArray(urls) ? urls : []) {
    const pairing = parseScan(url);
    if (pairing) return pairing;
  }
  return null;
}

/** How many computers the first screen remembers. */
export const RECENT_LIMIT = 5;

/**
 * The recent computers a saved list holds, newest first. Anything that cannot be read, or holds an
 * address that is not one, is left out, so a damaged list is an empty one and never a failure.
 */
export function readRecent(text) {
  let saved;
  try {
    saved = JSON.parse(String(text ?? "[]"));
  } catch {
    return [];
  }
  if (!Array.isArray(saved)) return [];
  const seen = new Set();
  const list = [];
  for (const entry of saved) {
    const answer = normalizeAddress(entry?.address);
    const lastUsed = Number(entry?.lastUsed);
    if (!answer.ok || !Number.isFinite(lastUsed) || seen.has(answer.address)) continue;
    seen.add(answer.address);
    list.push({ address: answer.address, lastUsed });
  }
  return list.sort((a, b) => b.lastUsed - a.lastUsed).slice(0, RECENT_LIMIT);
}

/** The list with this address first, as just used, and at most RECENT_LIMIT kept. */
export function remember(list, address, now) {
  return [{ address, lastUsed: now }, ...list.filter((entry) => entry.address !== address)].slice(
    0,
    RECENT_LIMIT,
  );
}

/** The list without this address. */
export function forget(list, address) {
  return list.filter((entry) => entry.address !== address);
}

/** The short name of a computer: the first part of its name on the tailnet, or its address. */
export function hostLabel(address) {
  const host = String(address ?? "").split(":")[0] ?? "";
  return /^\d+\.\d+\.\d+\.\d+$/.test(host) ? host : (host.split(".")[0] ?? host);
}
