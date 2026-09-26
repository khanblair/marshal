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
 *
 * Two kinds of topic. `topics` are the ones that do not depend on the store, such as `home`. A
 * syncer whose events arrive on a topic per project, such as a card's events on `project:<id>`,
 * gives `projectTopics`, and `startSync` subscribes to the topics of the projects that exist and
 * unsubscribes as they come and go.
 */
export interface Syncer<T = unknown> {
  section: SectionId;
  /** Event topics this section needs whatever the store holds, such as `home`. */
  topics: readonly string[];
  /** The topics this section needs for one project, or none when it has no per-project events. */
  projectTopics?(projectID: string): readonly string[];
  /**
   * Loads the snapshot. It is given the store as well as the client, because a section whose
   * snapshot is one answer per project (a board, for instance) has to read which projects exist.
   */
  load(api: ApiClient, ctx: Ctx): Promise<T>;
  apply(ctx: Ctx, snapshot: T): void;
  /** Called for every event of every subscribed topic; it picks its own by `event.type`. */
  onEvent?(ctx: Ctx, event: WireEvent): void;
  /**
   * Starts what the section does between snapshots, such as watching the store to save a change,
   * inside the reactive root `startSync` owns. It returns how to stop it. It runs once, when the
   * store starts following the daemon, and only for a section that is switched to the daemon.
   */
  start?(ctx: Ctx, api: ApiClient): () => void;
}
