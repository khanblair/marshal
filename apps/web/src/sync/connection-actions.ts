import type { Ctx } from "~/mock/context";

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
