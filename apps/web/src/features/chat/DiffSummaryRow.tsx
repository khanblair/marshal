import { Button, Icon } from "@marshal/ui";
import type { MsgView } from "~/mock";

export interface DiffSummaryRowProps {
  item: MsgView;
}

/** "Changed N files" with the line counts and a button that opens the card's Diff tab. */
export function DiffSummaryRow(props: DiffSummaryRowProps) {
  return (
    <div class="max-w-[72ch] flex items-center gap-2.5 min-h-9 py-1 pr-1.5 pl-2.5 border border-border rounded-sm">
      <Icon name="file-diff" size={14} />
      <span class="flex-1 font-medium">{props.item.summary}</span>
      <span class="font-mono text-caption text-diff-added-text">{props.item.add}</span>
      <span class="font-mono text-caption text-diff-removed-text">{props.item.del}</span>
      <Button size={28} onClick={() => props.item.openDiff?.()}>
        Open diff
      </Button>
    </div>
  );
}
