import { type JSX, splitProps } from "solid-js";

export interface SkeletonGroupProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /** What a screen reader hears for the whole group. Default `Loading`. */
  label?: string;
}

/**
 * Wraps every skeleton shape of one loading part in a single status, so a screen reader hears
 * one "Loading" and not one message per block. The shapes inside are `aria-hidden`. The caller
 * marks the container it is loading with `aria-busy="true"` and clears that when the data
 * arrives. The group adds no layout: give it `class` for that.
 */
export function SkeletonGroup(props: SkeletonGroupProps) {
  const [local, others] = splitProps(props, ["label", "class", "children"]);
  return (
    <div role="status" {...others} class={local.class}>
      <span class="sr-only">{local.label ?? "Loading"}</span>
      {local.children}
    </div>
  );
}
