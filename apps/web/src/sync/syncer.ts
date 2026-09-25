import type { Event as WireEvent } from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import type { SectionId } from "~/data/sections";
import type { Ctx } from "~/mock/context";

/**
 * One section that the daemon fills. `startSync` loads its snapshot when the app comes online (and
 * again after every return and every `Resync`), applies it in one batch, subscribes to its topics,
 * and hands it the events it cares about. Adding a section is adding one of these to `SYNCERS`.
 *
 * `apply` and `onEvent` must be idempotent: a snapshot and an event can describe the same change,
 * and the stream may send the same one twice.
 */
export interface Syncer<T = unknown> {
  section: SectionId;
  /** Event topics this section needs, such as `home`. */
  topics: readonly string[];
  load(api: ApiClient): Promise<T>;
  apply(ctx: Ctx, snapshot: T): void;
  /** Called for every event of every subscribed topic; it picks its own by `event.type`. */
  onEvent?(ctx: Ctx, event: WireEvent): void;
}
