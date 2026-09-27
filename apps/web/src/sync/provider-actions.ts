import { batch } from "solid-js";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";
import { applyProviderList } from "./providers";

/**
 * Saves a provider's secret on the daemon: an API key, or a local provider's server address. The
 * daemon checks the value, writes it to the OS keychain, and tests it as part of the same call, so
 * nothing is judged here: its refusal is shown as its own sentence (by `optimistic`), the store
 * keeps what it had, and this returns false so the form stays open.
 *
 * The key itself never comes back. The answer is the whole list with the stored value masked, and
 * that answer is what the store keeps, so a key exists only in the keychain and in the request.
 */
export async function saveProviderKey(ctx: Ctx, id: string, key: string): Promise<boolean> {
  const api = ctx.env.data?.api;
  const value = key.trim();
  if (!api || !value) return false;
  try {
    const list = await ctx.optimistic({
      key: `provider:${id}`,
      apply: () => undefined,
      request: () => api.saveProvider(id, { key: value }),
      rollback: () => undefined,
    });
    batch(() => {
      applyProviderList(ctx, list);
      toast(ctx, "Key saved");
    });
    return true;
  } catch {
    return false;
  }
}

/**
 * Runs one connection test now and mirrors what it found. A test that ran and found a bad key is a
 * success with a failed check, so only a test that could not run - or one asked for inside the
 * daemon's cooldown - comes back as a failure, and its own sentence is shown.
 *
 * After a test the list is read again, because a test can change a row's state (`invalid` after a
 * failed key check) and only the daemon decides that. The re-read is best effort: the test's own
 * result is already on the row if it fails.
 */
export async function testProviderKey(ctx: Ctx, id: string): Promise<boolean> {
  const api = ctx.env.data?.api;
  if (!api) return false;
  let ok: boolean;
  try {
    const result = await ctx.optimistic({
      key: `provider-test:${id}`,
      apply: () => undefined,
      request: () => api.testProvider(id),
      rollback: () => undefined,
    });
    ok = result.ok;
  } catch {
    return false;
  }
  try {
    const list = await api.listProviders();
    batch(() => applyProviderList(ctx, list));
  } catch {
    // The test ran; only the re-read failed, so the row keeps the state it had.
  }
  toast(ctx, ok ? "Connection test passed" : "Connection test failed");
  return ok;
}
