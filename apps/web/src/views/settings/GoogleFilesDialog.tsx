import { Button, type SegmentOption } from "@marshal/ui";
import { createSignal, Match, Show, Switch } from "solid-js";
import { GDRIVE_ID } from "~/sync/integrations";
import { ConnectionStored } from "./ConnectionStored";
import { ErrorLine } from "./connect-fields";
import { DriveFolder } from "./DriveFolder";
import { GoogleFilesList } from "./GoogleFilesList";
import {
  createGoogleFilesConnect,
  type GoogleFilesController,
  type GoogleFilesProps,
} from "./google-files-connect";
import { WaysDialog } from "./WaysDialog";

type Way = "access" | "files";

const WAYS: readonly SegmentOption<Way>[] = [
  { value: "access", label: "Google access" },
  { value: "files", label: "Files" },
];

const OPENS_GOOGLE = "Opens Google in a new tab. Use the computer that runs Marshal.";
const NO_CLIENT = "Save a Google client under Google Calendar first, then connect here.";

/** What the connection is for and what Google will be asked to allow, in the person's words. */
function PlainWords(props: { purpose: string; permission: string }) {
  return (
    <div class="flex flex-col gap-1 rounded-sm bg-surface-sunken p-3 text-small leading-4.5">
      <span>{props.purpose}</span>
      <span class="text-secondary">{props.permission}</span>
    </div>
  );
}

/** What can be pressed to make or renew the connection: Connect on a desktop, a sentence on a phone. */
function ConnectAction(props: { google: GoogleFilesController }) {
  const google = () => props.google;
  return (
    <Switch>
      <Match when={google().noClient()}>
        <span class="text-small leading-4.5 text-secondary">{NO_CLIENT}</span>
      </Match>
      <Match when={true}>
        <div class="flex flex-wrap items-center gap-2">
          <Button variant="primary" disabled={google().busy()} onClick={google().connect}>
            {google().busy() ? "Opening Google…" : "Connect with Google"}
          </Button>
          <span class="text-small text-secondary">{OPENS_GOOGLE}</span>
        </div>
      </Match>
    </Switch>
  );
}

/** The first tab: Google's access for this one connection, and what Marshal will and will not do. */
function AccessTab(
  props: { google: GoogleFilesController } & Pick<GoogleFilesProps, "spec" | "integration">,
) {
  const google = () => props.google;
  return (
    <div class="flex flex-col gap-3">
      <PlainWords purpose={props.spec.purpose} permission={props.spec.permission} />
      <ErrorLine message={google().error()} />
      <Show when={google().phone()}>
        <span class="text-small leading-4.5 text-secondary">
          Connect {props.spec.name} on the desktop app. This phone uses the connection once it is
          made.
        </span>
      </Show>
      <Show
        when={google().stored()}
        fallback={
          <Show when={!google().phone()}>
            <ConnectAction google={google()} />
          </Show>
        }
      >
        <ConnectionStored integration={props.integration} onDisconnected={() => undefined}>
          <Show when={!google().phone()}>
            <Button disabled={google().busy()} onClick={google().connect}>
              Reconnect {props.spec.name}
            </Button>
          </Show>
        </ConnectionStored>
      </Show>
    </div>
  );
}

/** The second tab: the files Marshal made, and for Drive the folder they go in. */
function FilesTab(props: { google: GoogleFilesController } & GoogleFilesProps) {
  const [folder, setFolder] = createSignal<string>();
  return (
    <div class="flex flex-col gap-3">
      <Show when={props.spec.id === GDRIVE_ID}>
        <DriveFolder saved={folder()} save={props.saveFolder} />
      </Show>
      <GoogleFilesList
        name={props.spec.name}
        kind={props.spec.kind}
        connected={props.google.connected()}
        load={props.load}
        onFolder={setFolder}
      />
    </div>
  );
}

/**
 * Connecting Google Drive, Docs, Sheets or Slides, as a dialog over Settings: Google's access for
 * that one connection, and the files Marshal made with it. One spec says what differs.
 */
export function GoogleFilesDialog(props: GoogleFilesProps) {
  const [way, setWay] = createSignal<Way>("access");
  const google = createGoogleFilesConnect(props);
  return (
    <WaysDialog
      titleId="google-files-title"
      title={`Connect ${props.spec.name}`}
      label={`${props.spec.name} settings`}
      ways={WAYS}
      value={way()}
      onValueChange={setWay}
      onClose={() => props.edit.close()}
    >
      <Switch>
        <Match when={way() === "access"}>
          <AccessTab spec={props.spec} integration={props.integration} google={google} />
        </Match>
        <Match when={way() === "files"}>
          <FilesTab {...props} google={google} />
        </Match>
      </Switch>
    </WaysDialog>
  );
}
