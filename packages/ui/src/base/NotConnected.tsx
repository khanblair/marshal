import { Show, splitProps } from "solid-js";
import { Button } from "./Button";
import { EmptyState, type EmptyStateProps } from "./EmptyState";

export interface NotConnectedProps
  extends Omit<EmptyStateProps, "icon" | "action" | "messageClass" | "children"> {
  /** The name of the service, such as `GitHub`. */
  service: string;
  /** One sentence on what connecting gives you. */
  reason?: string;
  /** Called when the person presses the connect button. */
  onConnect: () => void;
  /** Default `Connect {service}`. */
  connectLabel?: string;
}

/**
 * A service that is not set up. It shows no sample rows or numbers, only that the service is not
 * connected, why connecting helps, and the button that starts it. Built on `EmptyState`.
 */
export function NotConnected(props: NotConnectedProps) {
  const [local, others] = splitProps(props, ["service", "reason", "onConnect", "connectLabel"]);
  return (
    <EmptyState
      {...others}
      icon="plug"
      messageClass="max-w-[40ch]"
      action={
        <Button variant="primary" onClick={() => local.onConnect()}>
          {local.connectLabel ?? `Connect ${local.service}`}
        </Button>
      }
    >
      {local.service} is not connected.
      <Show when={local.reason}>{(reason) => <> {reason()}</>}</Show>
    </EmptyState>
  );
}
