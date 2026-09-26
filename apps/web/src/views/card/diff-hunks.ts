import { type Card, type DiffFile, M } from "~/mock";
import type { Panel } from "./panel-state";

type Hunks = DiffFile["hunks"];

export interface HunkLoader {
  /** A file's own hunks, once fetched, or the ones it already carried. */
  hunksOf: (file: DiffFile) => Hunks;
  isLoaded: (file: DiffFile) => boolean;
  /** Fetches a file's hunks (`M.loadFileHunks`) unless they are here or already on their way. */
  load: (file: DiffFile) => Promise<void>;
}

/**
 * The hunks of the open card's diff (section S11). They are fetched one file at a time, when the
 * screen needs them, and kept in the panel state so closing and opening a file, or another tab,
 * does not fetch them again. A file that is already being fetched is not asked for twice, and an
 * answer that lands after another card was opened is dropped.
 */
export function createHunkLoader(card: () => Card, panel: Panel): HunkLoader {
  const pending = new Set<string>();
  const hunksOf = (file: DiffFile): Hunks => panel.state.diffHunks[file.path] ?? file.hunks;
  const isLoaded = (file: DiffFile): boolean =>
    hunksOf(file).length > 0 || file.path in panel.state.diffHunks;
  const load = async (file: DiffFile): Promise<void> => {
    if (isLoaded(file) || pending.has(file.path)) return;
    const asked = card();
    pending.add(file.path);
    try {
      const hunks = await M.loadFileHunks(asked, file.path);
      if (card().id === asked.id)
        panel.set({ diffHunks: { ...panel.state.diffHunks, [file.path]: hunks } });
    } catch (error) {
      // The daemon's own sentence says what to do; the file stays as it was, so opening it again
      // (or pressing Load diff) asks again.
      M.toast(
        error instanceof Error ? error.message : "Marshal could not load that file's changes.",
      );
    } finally {
      pending.delete(file.path);
    }
  };
  return { hunksOf, isLoaded, load };
}
