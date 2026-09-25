import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export interface ChoiceCardProps extends JSX.ButtonHTMLAttributes<HTMLButtonElement> {
  selected: boolean;
}

/**
 * A large radio option (`role="radio"`) drawn as a card: a 2 px ink border
 * when selected, a 1 px light border otherwise. Put several in an element with
 * `role="radiogroup"`. Used for the theme cards and the first-project choice;
 * set the gap with a class (`gap-2.5`, or `gap-1.5 items-start`).
 */
export function ChoiceCard(props: ChoiceCardProps) {
  const [local, others] = splitProps(props, ["selected", "class"]);
  return (
    // biome-ignore lint/a11y/useSemanticElements: the design draws these radio options as cards; a native radio cannot hold that content
    <button
      type="button"
      role="radio"
      aria-checked={local.selected}
      {...others}
      class={cx(
        "flex flex-col p-3 rounded-lg bg-surface text-left",
        local.selected ? "border-2 border-ink" : "border border-border",
        local.class,
      )}
    />
  );
}
