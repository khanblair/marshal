/** The port the daemon serves on when the page's own address does not say. */
const DEFAULT_PORT = "47800";
const BARE_CODE = /^[A-Z0-9]{3,}-?[A-Z0-9]{3,}$/i;

/**
 * The QR code the desktop shows for a pairing: the address the phone reaches the daemon at, and the
 * code. The phone app's first screen reads the address; the page the daemon serves reads the code.
 */
export function pairingPayload(host: string, code: string): string {
  return `marshal://pair?host=${encodeURIComponent(host)}&code=${encodeURIComponent(code)}`;
}

/** The address a phone reaches this daemon at: its tailnet name, on the port the page came from. */
export function tailnetHost(dnsName: string, pagePort: string): string {
  return `${dnsName}:${pagePort || DEFAULT_PORT}`;
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
