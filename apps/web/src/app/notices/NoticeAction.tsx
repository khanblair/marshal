import { cx } from "@marshal/ui";
import type { NoticeActionModel } from "./notice-list";

export interface NoticeActionProps {
  action: NoticeActionModel;
  /** `row` is the small button beside a card row (12 px text); `card` is the footer button (13 px). */
  size: "row" | "card";
}

/** One outlined button of a notice; the primary action is filled with ink. */
export function NoticeAction(props: NoticeActionProps) {
  return (
    <button
      type="button"
      onClick={() => props.action.run()}
      class={cx(
        "h-7 rounded-sm border font-semibold hover:opacity-90",
        props.size === "row" ? "px-2 text-caption" : "px-2.5 text-small",
        props.action.primary
          ? "border-ink bg-ink text-on-ink"
          : "border-border-strong bg-surface text-primary",
      )}
    >
      {props.action.label}
    </button>
  );
}
