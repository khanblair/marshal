import { Button, IconButton, TextArea } from "@marshal/ui";
import { M } from "~/mock";
import { addHint, addLabel, type ColumnRef } from "./board-model";
import { boardPid, boardSwim } from "./board-state";
import { laneExtraOf, openTemplateCard, submitQuickAdd } from "./quick-add";

const FOCUS_DELAY_MS = 0;

/** The inline title form of a column. Enter adds the card and keeps the form open; Escape closes it. */
export function QuickAddForm(props: ColumnRef) {
  let field: HTMLTextAreaElement | undefined;
  const extra = () => laneExtraOf(boardSwim(), props.laneKey);
  const submit = (): void => {
    if (!field) return;
    submitQuickAdd(boardPid(), props.col, field.value, extra());
    field.value = "";
    field.focus();
  };
  // A native listener, not a delegated one: Escape must not reach the shell's window handler.
  const onKeyDown = (e: KeyboardEvent): void => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      submit();
    }
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      M.set({ quickAddAt: null });
    }
  };
  return (
    <form
      class="flex flex-col gap-1.5"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <TextArea
        name="title"
        rows={2}
        ref={(el) => {
          field = el;
          setTimeout(() => el.focus(), FOCUS_DELAY_MS);
        }}
        on:keydown={onKeyDown}
        placeholder="Enter a title for this card"
        aria-label="Card title"
        class="rounded-md! resize-none!"
      />
      <span class="text-caption leading-4 text-secondary">{addHint(props.col)}</span>
      <div class="flex items-center gap-1.5">
        <Button variant="primary" type="submit">
          {addLabel(props.col)}
        </Button>
        <IconButton
          label="Cancel"
          icon="x"
          size={32}
          tone="default"
          onClick={() => M.set({ quickAddAt: null })}
        />
        <span class="flex-1" />
        <IconButton
          label="Create from a template"
          title="Create from a template"
          icon="copy-plus"
          size={32}
          onClick={() => openTemplateCard(props.col, extra())}
        />
      </div>
    </form>
  );
}
