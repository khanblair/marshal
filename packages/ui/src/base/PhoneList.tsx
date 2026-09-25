import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export type PhoneListProps = JSX.HTMLAttributes<HTMLUListElement>;

/** The list that replaces a table on phones: 16 px sides, 24 px below, no bullets. */
export function PhoneList(props: PhoneListProps) {
  const [local, others] = splitProps(props, ["class", "children"]);
  return (
    <ul {...others} class={cx("m-0 px-4 pb-6 list-none", local.class)}>
      {local.children}
    </ul>
  );
}
