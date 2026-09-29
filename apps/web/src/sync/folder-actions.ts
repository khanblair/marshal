import type { FolderListing } from "@marshal/protocol";
import { ApiError } from "~/data/api-error";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";

const NOT_CONNECTED = "Marshal is not connected to its daemon.";
const GENERIC_FAILURE = "Marshal could not open that folder. Try again.";

/**
 * The folders inside one folder of the computer the daemon runs on, home when no path is given, for
 * picking a repository from a page that cannot open the system's own folder dialog. A refusal is shown
 * as the daemon's own sentence and answers null.
 */
export async function browseFolders(ctx: Ctx, path?: string): Promise<FolderListing | null> {
  const api = ctx.env.data?.api;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return null;
  }
  try {
    return await api.folders(path);
  } catch (error) {
    toast(ctx, error instanceof ApiError ? error.message : GENERIC_FAILURE);
    return null;
  }
}
