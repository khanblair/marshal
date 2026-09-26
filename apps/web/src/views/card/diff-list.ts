import { type Accessor, createMemo, createResource } from "solid-js";
import { type Card, type DiffFile, M } from "~/mock";

export interface DiffList {
  /** The changed files with their counts, empty until the first answer lands or after a failure. */
  files: Accessor<DiffFile[]>;
  /** The first answer has not landed, or a new one is on its way. */
  loading: Accessor<boolean>;
  /** Why the last answer failed, when it did. */
  error: Accessor<unknown>;
  /** True once an answer has landed, so the list can be asked for again when a tab opens. */
  ready: Accessor<boolean>;
  refetch: () => void;
}

/**
 * The open card's changed files (section S11): the daemon's own list, without hunks, once it is
 * switched and this card is one it knows; the mock's own `diffFor` otherwise, which already
 * carries every file's hunks. The card panel makes one, so the Diff tab and the tab bar's count
 * read the same answer, and the daemon is asked once for the card and not once for each.
 *
 * It is asked again only for another card (by its key: the store may hand out a new object for
 * the same card, and a card that changes every few seconds must not refetch a list of thousands),
 * or when the caller says so.
 */
export function createDiffList(card: () => Card): DiffList {
  const [remote, { refetch }] = createResource(
    () => card().id,
    () => M.loadCardDiff(card()),
  );
  // Reading the resource after a failure throws, so the error is looked at first.
  const files = createMemo<DiffFile[]>(() => (remote.error ? [] : (remote() ?? [])));
  return {
    files,
    loading: () => remote.loading,
    error: () => remote.error,
    ready: () => remote.state === "ready",
    refetch: () => void refetch(),
  };
}
