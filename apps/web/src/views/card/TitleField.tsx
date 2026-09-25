import { onMount } from "solid-js";
import { M } from "~/mock";
import type { Panel } from "./panel-state";

export interface TitleFieldProps {
  cardId: number;
  title: string;
  panel: Panel;
}

/** The card title as a text field: Enter saves, Escape cancels, leaving the field saves. */
export function TitleField(props: TitleFieldProps) {
  let input: HTMLInputElement | undefined;
  onMount(() => {
    input?.focus();
    input?.select();
  });
  const done = () => props.panel.set({ editingTitle: false });
  const onKeyDown = (e: KeyboardEvent & { currentTarget: HTMLInputElement }) => {
    if (e.key === "Enter") {
      M.rename(props.cardId, e.currentTarget.value);
      done();
    }
    if (e.key === "Escape") {
      e.stopPropagation();
      e.preventDefault();
      done();
    }
  };
  const onBlur = (e: FocusEvent & { currentTarget: HTMLInputElement }) => {
    if (!props.panel.state.editingTitle) return;
    M.rename(props.cardId, e.currentTarget.value);
    done();
  };
  return (
    <input
      ref={input}
      value={props.title}
      onKeyDown={onKeyDown}
      onBlur={onBlur}
      aria-label="Card title"
      class="text-title leading-6 font-semibold py-0.5 px-1.5 -mx-1.75 rounded-sm border border-border-strong bg-surface"
    />
  );
}
