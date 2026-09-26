import { createStore } from "solid-js/store";
import type { Attachment, DiffFile } from "~/mock";

/**
 * Everything the card panel remembers about the open card while you switch tabs: the
 * comment draft, which members menu is open, which diff files are expanded, and so on.
 * A new card gets a new panel state, as the design resets these fields on a card change.
 */
interface PanelState {
  cDraft: string;
  pending: Attachment[];
  linkOpen: boolean;
  membersOpen: boolean;
  addingIn: string | null;
  newCl: boolean;
  editingTitle: boolean;
  draft: string;
  away: boolean;
  unseen: boolean;
  more: boolean;
  /** Paths of the expanded diff files, or null until the user expands or collapses one. */
  openFiles: string[] | null;
  loaded: Record<string, boolean>;
  /** A file's hunks once loaded (section S11): the daemon's own, fetched when the file was opened. */
  diffHunks: Record<string, DiffFile["hunks"]>;
  noteEdit: boolean;
  noteDraft: string;
}

const initialState = (): PanelState => ({
  cDraft: "",
  pending: [],
  linkOpen: false,
  membersOpen: false,
  addingIn: null,
  newCl: false,
  editingTitle: false,
  draft: "",
  away: false,
  unseen: false,
  more: false,
  openFiles: null,
  loaded: {},
  diffHunks: {},
  noteEdit: false,
  noteDraft: "",
});

export interface Panel {
  state: PanelState;
  /** Shallow merge, like the design's `setState`. */
  set: (patch: Partial<PanelState>) => void;
}

export function createPanelState(): Panel {
  const [state, setState] = createStore<PanelState>(initialState());
  return { state, set: (patch) => setState(patch) };
}
