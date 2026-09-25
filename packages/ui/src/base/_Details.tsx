import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

/* Private: the expandable "Details" of ConnectionLost and ErrorState (ui-rules 5.3). */

interface DetailsProps extends JSX.HTMLAttributes<HTMLDetailsElement> {
  /** The technical text: an error code and a short reason. Never a stack trace. */
  text: string;
}

/**
 * A native disclosure: the word "Details", and the technical text under it once opened. The
 * text wraps at any character so a long path or address never scrolls the page sideways at
 * 320 px, and a long text scrolls inside its own box.
 */
export function Details(props: DetailsProps) {
  const [local, others] = splitProps(props, ["text", "class"]);
  return (
    <details {...others} class={cx("w-full max-w-[460px] text-left", local.class)}>
      {/* The summary is not a button, so the base CSS rule that makes buttons 44 px high on touch screens does not reach it. */}
      <summary class="cursor-pointer py-1.5 text-small text-secondary [[data-touch='1']_&]:py-3">
        Details
      </summary>
      <pre class="m-0 mt-1.5 p-3 max-h-40 overflow-auto rounded-md bg-surface-sunken font-mono text-caption leading-4.5 whitespace-pre-wrap [overflow-wrap:anywhere]">
        {local.text}
      </pre>
    </details>
  );
}
