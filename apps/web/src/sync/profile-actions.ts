import type { UpdateProfileRequest } from "@marshal/protocol";
import { batch } from "solid-js";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";
import { applyProfile } from "./profile";

/** The fields of the Profile form, as the screen holds them. */
export interface ProfileFields {
  name: string;
  email: string;
  tz: string;
}

/** The fields that differ from what the store has, for the daemon's merge: what is not sent is not touched. */
function changesOf(ctx: Ctx, fields: ProfileFields): UpdateProfileRequest {
  const { profile } = ctx.S;
  const change: UpdateProfileRequest = {};
  const name = fields.name.trim();
  const email = fields.email.trim();
  if (name !== profile.name) change.name = name;
  if (email !== profile.email) change.email = email;
  if (fields.tz !== profile.tz) change.timeZone = fields.tz;
  return change;
}

/**
 * Saves the Profile form on the daemon and puts what it answered in the store. The daemon checks
 * every field, so nothing is judged here: its refusal is shown as its own sentence (by `optimistic`),
 * the store keeps what it had, and this returns false so the form keeps what was typed.
 */
export async function saveProfile(ctx: Ctx, fields: ProfileFields): Promise<boolean> {
  const api = ctx.env.data?.api;
  const change = changesOf(ctx, fields);
  if (!api) return false;
  if (Object.keys(change).length === 0) return true;
  try {
    const profile = await ctx.optimistic({
      apply: () => undefined,
      request: () => api.updateMe(change),
      rollback: () => undefined,
    });
    batch(() => {
      applyProfile(ctx, profile);
      toast(ctx, "Profile saved");
    });
    return true;
  } catch {
    return false;
  }
}

/** Sends the picked image as the avatar. The daemon's size and type sentences are shown as they are. */
async function uploadAvatar(ctx: Ctx, image: File): Promise<void> {
  const api = ctx.env.data?.api;
  if (!api) return;
  try {
    const profile = await ctx.optimistic({
      apply: () => undefined,
      request: () => api.uploadAvatar(image),
      rollback: () => undefined,
    });
    batch(() => {
      applyProfile(ctx, profile);
      toast(ctx, "Avatar saved");
    });
  } catch {
    // The daemon refused, and `optimistic` showed its sentence.
  }
}

/** Opens the file chooser and uploads the image that is picked. Closing the chooser changes nothing. */
export async function chooseAvatar(ctx: Ctx): Promise<void> {
  const image = await ctx.env.pickImage?.();
  if (image) await uploadAvatar(ctx, image);
}
