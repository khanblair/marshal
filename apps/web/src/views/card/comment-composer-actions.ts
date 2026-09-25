import { M } from "~/mock";
import type { CommentComposerProps } from "./CommentComposer";
import { attachmentsFrom, linkAttachment } from "./comment-model";

/** The link field only exists after the toggle renders it, so focus waits a moment. */
const LINK_FOCUS_DELAY_MS = 30;

/** What the new-comment box does: post, add a link or files, and the link field's keys. */
export function createComposerActions(
  props: CommentComposerProps,
  linkInput: () => HTMLInputElement | undefined,
) {
  const state = () => props.panel.state;
  const empty = () => !state().cDraft.trim() && !state().pending.length;
  const post = () => {
    M.addComment(props.cardId, state().cDraft, state().pending);
    props.panel.set({ cDraft: "", pending: [], linkOpen: false });
  };
  const addLink = () => {
    const value = linkInput()?.value.trim();
    if (!value) return;
    props.panel.set({ pending: [...state().pending, linkAttachment(value)], linkOpen: false });
  };
  const onFiles = (e: Event & { currentTarget: HTMLInputElement }) => {
    props.panel.set({ pending: [...state().pending, ...attachmentsFrom(e.currentTarget.files)] });
    e.currentTarget.value = "";
  };
  const toggleLink = () => {
    props.panel.set({ linkOpen: !state().linkOpen });
    setTimeout(() => linkInput()?.focus(), LINK_FOCUS_DELAY_MS);
  };
  const onLinkKey = (e: KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      addLink();
    }
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      props.panel.set({ linkOpen: false });
    }
  };
  return { state, empty, post, addLink, onFiles, toggleLink, onLinkKey };
}
