import type { SegmentOption } from "@marshal/ui";
import { createEffect, createSignal, Match, Switch } from "solid-js";
import { GoogleOwnClient } from "./GoogleOwnClient";
import { GoogleSignIn } from "./GoogleSignIn";
import { createGoogleConnect, type GoogleConnectProps } from "./google-connect";
import { WaysDialog } from "./WaysDialog";

type Way = "sign-in" | "own";

const WAYS: readonly SegmentOption<Way>[] = [
  { value: "sign-in", label: "Sign in with Google" },
  { value: "own", label: "Use my own client" },
];

/**
 * Connecting Google Calendar (B8.3), as a dialog over Settings: a one-click sign-in with Marshal's
 * own Google client, or a client of the person's own. Both tabs read the one connection, so what one
 * connects, the other shows. It opens on the tab that can work: the person's own when this build has
 * no client of its own, or they saved one.
 */
export function GoogleConnectDialog(props: GoogleConnectProps) {
  const google = createGoogleConnect(props);
  const [way, setWay] = createSignal<Way>("sign-in");
  let chosen = false;
  createEffect(() => {
    if (google.info() && !chosen && !google.oneClick()) setWay("own");
  });
  const choose = (next: Way): void => {
    chosen = true;
    setWay(next);
  };
  return (
    <WaysDialog
      titleId="google-title"
      title="Connect Google Calendar"
      label="How to connect Google Calendar"
      ways={WAYS}
      value={way()}
      onValueChange={choose}
      onClose={() => props.edit.close()}
    >
      <Switch>
        <Match when={way() === "sign-in"}>
          <GoogleSignIn integration={props.integration} google={google} />
        </Match>
        <Match when={way() === "own"}>
          <GoogleOwnClient integration={props.integration} google={google} />
        </Match>
      </Switch>
    </WaysDialog>
  );
}
