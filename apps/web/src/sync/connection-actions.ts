import { DeviceKindMobile, DeviceKindWeb } from "@marshal/protocol";
import { ApiError } from "~/data/api-error";
import type { Ctx } from "~/mock/context";
import { platform } from "~/platform";

const HTTP_UNAUTHORIZED = 401;
const REFUSED_CODE =
  "That code did not work. Check it, or ask for a new one in Settings, Remote control.";
const NAME_LIMIT = 60;
const NOT_CONNECTED = "Marshal cannot reach the computer. Check the connection and try again.";

/** Shows the sign-in screen's field as busy and hands the token to the connection, which checks it. */
export function signIn(ctx: Ctx, token: string): void {
  const { data } = ctx.env;
  const { connection } = ctx.S;
  const trimmed = token.trim();
  if (!data || !connection || !trimmed) return;
  ctx.S.connection = { ...connection, busy: true };
  data.tokens.set(trimmed);
  // `set` starts a check by itself when the token changed; this covers a token that did not.
  data.connection.retryNow();
}

/** "Try again": loads the data again when only that failed, else checks the daemon at once. */
export function reconnect(ctx: Ctx): void {
  if (ctx.S.loadError && ctx.S.connection?.state === "online") {
    void ctx.sync?.reload();
    return;
  }
  ctx.env.data?.connection.retryNow();
}

/**
 * Trades a pairing code for this device's own token and signs in with it. It answers the sentence to
 * show under the field, or an empty string once the token is kept. The daemon refuses a bad, spent, or
 * expired code with one and the same 401, so that is put in the words a person can act on.
 */
export async function pairWithCode(ctx: Ctx, code: string, name: string): Promise<string> {
  const { data } = ctx.env;
  if (!data) return NOT_CONNECTED;
  try {
    const paired = await data.api.pairDevice({
      code: code.trim(),
      name: name.trim().slice(0, NAME_LIMIT),
      kind: platform().kind === "mobile" ? DeviceKindMobile : DeviceKindWeb,
    });
    data.tokens.set(paired.token);
    data.connection.retryNow();
    return "";
  } catch (error) {
    if (error instanceof ApiError && error.status === HTTP_UNAUTHORIZED) return REFUSED_CODE;
    return error instanceof ApiError ? error.message : NOT_CONNECTED;
  }
}
