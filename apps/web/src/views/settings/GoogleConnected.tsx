import { Button } from "@marshal/ui";
import { ConnectionStored } from "./ConnectionStored";
import { GoogleCalendarPicker } from "./GoogleCalendarPicker";
import type { GoogleConnectController, GoogleConnectProps } from "./google-connect";

/**
 * What a connected Google Calendar offers on either tab: Reconnect, the stored connection's own
 * test, Disconnect, and the choice of which calendars Marshal reads.
 */
export function GoogleConnected(props: {
  integration: GoogleConnectProps["integration"];
  google: GoogleConnectController;
}) {
  return (
    <>
      <ConnectionStored integration={props.integration} onDisconnected={() => undefined}>
        <Button disabled={props.google.busy()} onClick={props.google.connect}>
          Reconnect Google Calendar
        </Button>
      </ConnectionStored>
      <GoogleCalendarPicker connected={props.google.connected()} />
    </>
  );
}
