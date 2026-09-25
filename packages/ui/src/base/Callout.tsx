import { type JSX, Show, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import type { IconNameInput } from "../icons/icon-names";
import { cx } from "./cx";

export interface CalloutProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /** `warning` (default): needs-you tint. `neutral`: sunken fill, secondary text. */
  tone?: "warning" | "neutral";
  /** Icon at 14 px before the content. */
  icon?: IconNameInput;
}

const ICON_PX = 14;

/**
 * A tinted status box in 13 px text (`role="status"`): a possible duplicate
 * card, a weak model warning, or what Marshal detected in a repository.
 * Add `items-center` for a single line.
 */
export function Callout(props: CalloutProps) {
  const [local, others] = splitProps(props, ["tone", "icon", "class", "children"]);
  return (
    <div
      role="status"
      {...others}
      class={cx(
        "flex gap-2 py-2.5 px-3 rounded-md text-small leading-4.5",
        local.tone === "neutral"
          ? "bg-surface-sunken text-secondary"
          : "bg-status-needs-you-subtle text-status-needs-you-text",
        local.class,
      )}
    >
      <Show when={local.icon}>{(name) => <Icon name={name()} size={ICON_PX} />}</Show>
      {local.children}
    </div>
  );
}
