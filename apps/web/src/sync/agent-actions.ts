import { ApiError } from "~/data/api-error";
import { type ProviderTest, toProviderTest } from "~/data/mappers/providers";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";
import { applyAgentCatalog } from "./agents";

/*
 * The two things the Connect your agents screen does beyond showing the list: look for the agent
 * programs again (a scan), and test one of them. Both are the daemon's answer as it is; a refusal is
 * shown as its own sentence. A test starts the program and asks it to introduce itself, and sends no
 * prompt, so it costs nothing.
 */

const NOT_CONNECTED = "Marshal is not connected to its daemon.";
const GENERIC_FAILURE = "Marshal could not finish that. Try again.";

const sentenceOf = (error: unknown): string =>
  error instanceof ApiError ? error.message : GENERIC_FAILURE;

/** Looks for the agents again and puts the new list in the store. True when the daemon answered. */
export async function scanAgents(ctx: Ctx): Promise<boolean> {
  const api = ctx.env.data?.api;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return false;
  }
  try {
    applyAgentCatalog(ctx, await api.refreshAgents());
    return true;
  } catch (error) {
    toast(ctx, sentenceOf(error));
    return false;
  }
}

/** Tests one agent or tool by its id and answers what each check found, or null when it could not be asked. */
export async function testAgent(ctx: Ctx, id: string): Promise<ProviderTest | null> {
  const api = ctx.env.data?.api;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return null;
  }
  try {
    return toProviderTest(await api.testAgent(id));
  } catch (error) {
    toast(ctx, sentenceOf(error));
    return null;
  }
}
