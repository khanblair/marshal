import { Show, splitProps } from "solid-js";
import { Details } from "./_Details";
import { Button } from "./Button";
import { EmptyState, type EmptyStateProps } from "./EmptyState";

export interface ErrorStateProps extends Omit<EmptyStateProps, "icon" | "action" | "children"> {
  /** What went wrong and how to fix it, as one plain sentence from the caller. */
  message: string;
  /** Technical text (an error code and a short reason), shown only inside "Details". */
  details?: string;
  /** Shows a Try again button when given. */
  onRetry?: () => void;
}

/**
 * One section that failed to load, in the space the section would fill. It shows the caller's
 * message, a Try again button when there is something to retry, and the technical text inside
 * an expandable "Details". It is plain text and not an alert, because several sections can fail
 * at once and each would interrupt a screen reader. Use it for a section, not for the whole app:
 * an app that cannot reach the daemon shows `ConnectionLost`.
 */
export function ErrorState(props: ErrorStateProps) {
  const [local, others] = splitProps(props, ["message", "details", "onRetry"]);
  return (
    <EmptyState
      messageClass="max-w-[40ch]"
      {...others}
      icon="triangle-alert"
      action={
        <>
          <Show when={local.onRetry}>
            <Button onClick={() => local.onRetry?.()}>Try again</Button>
          </Show>
          <Show when={local.details}>{(text) => <Details text={text()} />}</Show>
        </>
      }
    >
      {local.message}
    </EmptyState>
  );
}
