import { type JSX, Show, splitProps } from "solid-js";
import { cx } from "../base/cx";

/** Width in px the text takes before the row around it wraps. */
export type ItemTextBasis = 160 | 180 | 220 | 240;

export interface ItemTextProps extends Omit<JSX.HTMLAttributes<HTMLSpanElement>, "title"> {
  /** Bold first line: a device, provider, schedule, or help topic. */
  title: JSX.Element;
  /** Secondary 13 px line under the title. */
  description?: JSX.Element;
  /** A second secondary line, such as a schedule's action. */
  detail?: JSX.Element;
  basis: ItemTextBasis;
  /** 18 px lines for the secondary text. Without it they inherit 20 px. */
  tight?: boolean;
}

const BASIS: Record<ItemTextBasis, string> = {
  160: "flex-[1_1_160px]",
  180: "flex-[1_1_180px]",
  220: "flex-[1_1_220px]",
  240: "flex-[1_1_240px]",
};

/**
 * A title over one or two lines of secondary text, growing to fill a wrapping
 * settings row next to its icon and buttons.
 */
export function ItemText(props: ItemTextProps) {
  const [local, others] = splitProps(props, [
    "title",
    "description",
    "detail",
    "basis",
    "tight",
    "class",
  ]);
  const lineClass = () => cx("text-small text-secondary", local.tight && "leading-4.5");
  return (
    <span {...others} class={cx("flex flex-col", BASIS[local.basis], local.class)}>
      <span class="font-semibold">{local.title}</span>
      <Show when={local.description}>
        <span class={lineClass()}>{local.description}</span>
      </Show>
      <Show when={local.detail}>
        <span class={lineClass()}>{local.detail}</span>
      </Show>
    </span>
  );
}
