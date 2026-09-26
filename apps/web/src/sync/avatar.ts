import type { ApiClient } from "~/data/api-client";
import type { Ctx } from "~/mock/context";

/**
 * The avatar picture the screens draw. The daemon serves it at an address that needs the token, and
 * an `<img>` cannot send one, so it is fetched with the client and drawn from an object URL. That
 * URL is the memory of a picture, so it is revoked when the picture is replaced and when the store
 * stops following the daemon.
 */
export interface AvatarPicture {
  /** Shows the picture at the daemon's `avatarUrl`, or none (the initials) for null. The same address twice fetches once. */
  show(avatarUrl: string | null): void;
  /** Revokes the object URL in use. Nothing is drawn from it afterwards. */
  stop(): void;
}

export function createAvatarPicture(ctx: Ctx, api: ApiClient): AvatarPicture {
  /** The daemon address the store shows, or null. The address carries the version, so a new image is a new address. */
  let shown: string | null = null;
  let objectUrl: string | null = null;
  // Only the newest request may put a picture on screen: a slow answer for an old one is dropped.
  let newest = 0;

  const release = (): void => {
    if (objectUrl) URL.revokeObjectURL(objectUrl);
    objectUrl = null;
  };

  async function fetchPicture(avatarUrl: string, mine: number): Promise<void> {
    try {
      const image = await api.avatarImage(avatarUrl);
      if (mine !== newest) return;
      const next = URL.createObjectURL(image);
      const old = objectUrl;
      objectUrl = next;
      ctx.S.profile.avatar = next;
      if (old) URL.revokeObjectURL(old);
    } catch {
      // The initials are drawn in place of a picture that cannot be read, and the next answer tries again.
      if (mine !== newest) return;
      shown = null;
      ctx.S.profile.avatar = null;
      release();
    }
  }

  return {
    show(avatarUrl) {
      if (avatarUrl === shown) return;
      shown = avatarUrl;
      newest += 1;
      if (avatarUrl === null) {
        ctx.S.profile.avatar = null;
        release();
        return;
      }
      void fetchPicture(avatarUrl, newest);
    },
    stop() {
      newest += 1;
      shown = null;
      release();
      ctx.S.profile.avatar = null;
    },
  };
}
