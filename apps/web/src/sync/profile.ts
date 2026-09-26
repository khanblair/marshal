import {
  EventTypeMeUpdated,
  MeTopic,
  type Timestamp,
  type Event as WireEvent,
  type Profile as WireProfile,
  type User as WireUser,
} from "@marshal/protocol";
import { batch } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { isRecord } from "~/data/guards";
import type { Ctx } from "~/mock/context";
import { type AvatarPicture, createAvatarPicture } from "./avatar";
import { toPerson, toStoredProfile } from "./me-mapper";
import type { Syncer } from "./syncer";

/**
 * Section S2a: the person. Their profile (name, email, time zone, initials, avatar) is `S.profile`,
 * and everyone who can be put on a card is `S.people`. Solo use lists the owner only (decision D10).
 * The tailnet, the node, and the paired devices are not the daemon's until Phase 9, so they are
 * left as they are.
 *
 * The profile changes as `me.updated`, which carries the whole profile, so applying it twice
 * changes nothing.
 */

interface PersonSnapshot {
  profile: WireProfile;
  users: WireUser[];
}

/** The picture of each store that follows the daemon, so an event can change it. */
const pictures = new WeakMap<Ctx, AvatarPicture>();

/** The wire's `updatedAt` of the newest profile this store has applied, so an older one never wins. */
const appliedAt = new WeakMap<Ctx, Timestamp>();

/**
 * Writes the daemon's profile into the store, one field at a time, so nothing redraws that did not
 * change.
 *
 * `snapshot` says the profile was read at load. The daemon bumps `updatedAt` on every profile change,
 * including the avatar, so a snapshot read before an event that was already applied carries the older
 * time, and putting it over the newer profile would undo that change (an avatar that is there again,
 * for example). Only the load can be stale: an event, and the answer to a write this client just
 * made, are the newest state the daemon has.
 */
export function applyProfile(ctx: Ctx, wire: WireProfile, snapshot = false): void {
  const at = wire.updatedAt;
  const newest = appliedAt.get(ctx);
  if (snapshot && at && newest && at < newest) return;
  if (at) appliedAt.set(ctx, at);
  const { S } = ctx;
  batch(() => {
    for (const [key, value] of Object.entries(toStoredProfile(wire))) {
      if (S.profile[key as keyof typeof S.profile] !== value)
        Object.assign(S.profile, { [key]: value });
    }
    // The person is also in the list the member pickers read, under the same id.
    const listed = S.people.find((person) => person.id === wire.id);
    if (listed && listed.name !== wire.name) listed.name = wire.name;
  });
  pictures.get(ctx)?.show(wire.avatarUrl ?? null);
}

/** Makes the people the daemon's list, in its order. */
function applyPeople(ctx: Ctx, users: readonly WireUser[]): void {
  ctx.S.people = users.map(toPerson);
}

function onProfileEvent(ctx: Ctx, event: WireEvent): void {
  if (event.type !== EventTypeMeUpdated || !isRecord(event.data)) return;
  if (isRecord(event.data.profile)) applyProfile(ctx, event.data.profile as unknown as WireProfile);
}

export const profileSyncer: Syncer<PersonSnapshot> = {
  section: "S2a",
  topics: [MeTopic],
  async load(api: ApiClient) {
    const [profile, users] = await Promise.all([api.me(), api.users()]);
    return { profile, users: users.users };
  },
  apply(ctx, snapshot) {
    batch(() => {
      applyPeople(ctx, snapshot.users);
      applyProfile(ctx, snapshot.profile, true);
    });
  },
  onEvent: onProfileEvent,
  start(ctx, api) {
    const picture = createAvatarPicture(ctx, api);
    pictures.set(ctx, picture);
    return () => {
      picture.stop();
      pictures.delete(ctx);
    };
  },
};
