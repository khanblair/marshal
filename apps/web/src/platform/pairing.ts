import type { TailnetStatus } from "@marshal/protocol";
import { phoneAccess } from "./phone-access";

const BARE_CODE = /^[A-Z0-9]{3,}-?[A-Z0-9]{3,}$/i;

/**
 * The QR code the desktop shows for a pairing: the address the phone reaches the daemon at, and the
 * code. The phone app's first screen reads the address; the page the daemon serves reads the code.
 */
export function pairingPayload(host: string, code: string): string {
  return `marshal://pair?host=${encodeURIComponent(host)}&code=${encodeURIComponent(code)}`;
}

/**
 * The text of the QR code for a live pairing code, or null while there is no address a phone can use:
 * the daemon's own node has to be online, or Tailscale on this computer has to hand the daemon a port
 * and have been seen to answer there (see `phoneAccess`).
 */
export function pairingQrText(
  status: TailnetStatus | null | undefined,
  code: string | undefined,
): string | null {
  const access = phoneAccess(status);
  if (!code || !access?.address || access.next) return null;
  return pairingPayload(access.address, code);
}

/**
 * The pairing code in what a scan read: the desktop's QR payload, or a bare code such as `7QX-2LD`.
 * Anything else is not a code, so a photo of something else never pairs.
 */
export function parsePairingScan(text: string | null): string | null {
  const scanned = (text ?? "").trim();
  if (scanned.startsWith("marshal://")) {
    try {
      const url = new URL(scanned);
      const code = url.hostname === "pair" ? (url.searchParams.get("code") ?? "").trim() : "";
      return code || null;
    } catch {
      return null;
    }
  }
  return BARE_CODE.test(scanned) ? scanned : null;
}
