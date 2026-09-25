import { Button, IconButton } from "@marshal/ui";
import { M } from "~/mock";
import type { ColumnRef } from "./board-model";
import { boardSwim } from "./board-state";
import { laneExtraOf, openTemplateCard } from "./quick-add";

/** "Add a card" at the bottom of a column, with a shortcut to the template dialog. */
export function AddCardRow(props: ColumnRef) {
  return (
    <div class="flex items-center gap-0.5">
      <Button
        variant="quiet"
        icon="plus"
        class="flex-1 justify-start! px-2!"
        onClick={() => M.set({ quickAddAt: `${props.laneKey}:${props.col}` })}
      >
        Add a card
      </Button>
      <IconButton
        label="Create from a template"
        title="Create from a template"
        icon="copy-plus"
        size={32}
        onClick={() => openTemplateCard(props.col, laneExtraOf(boardSwim(), props.laneKey))}
      />
    </div>
  );
}
