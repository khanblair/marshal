import { cx } from "@marshal/ui";
import type { JSX } from "solid-js";

export interface StepIntroProps {
  class?: string;
  children: JSX.Element;
}

/** The secondary line of text under each screen's title. */
export function StepIntro(props: StepIntroProps) {
  return <p class={cx("m-0 text-secondary", props.class)}>{props.children}</p>;
}
