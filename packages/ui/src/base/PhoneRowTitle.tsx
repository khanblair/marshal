import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export interface PhoneRowTitleProps extends Omit<JSX.HTMLAttributes<HTMLSpanElement>, "title"> {
  title: JSX.Element;
  /** The card number, such as `#41`. Empty for the Orchestrator. */
  num: string;
  /** A color the store chose for the title (quiet for done or asleep cards). */
  titleColor?: string;
}

/** First line of a phone list row: bold title, then the number in muted text. */
export function PhoneRowTitle(props: PhoneRowTitleProps) {
  const [local, others] = splitProps(props, ["title", "num", "titleColor", "class"]);
  return (
    <span {...others} class={cx("flex gap-2 items-baseline", local.class)}>
      <span class="font-semibold" style={{ color: local.titleColor }}>
        {local.title}
      </span>
      <span class="text-small text-muted">{local.num}</span>
    </span>
  );
}
