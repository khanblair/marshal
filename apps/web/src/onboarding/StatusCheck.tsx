import { Icon } from "@marshal/ui";
import type { JSX } from "solid-js";

export interface StatusCheckProps {
  children: JSX.Element;
}

/** A green check and a word: Found for an agent, Connected for a chat app. */
export function StatusCheck(props: StatusCheckProps) {
  return (
    <span class="inline-flex items-center gap-1 text-small font-semibold text-status-working-text">
      <Icon name="check" size={14} />
      {props.children}
    </span>
  );
}
