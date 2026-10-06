import type { GoogleFile, GoogleFileKind, GoogleFiles } from "@marshal/protocol";
import { Button } from "@marshal/ui";
import { createEffect, createResource, For, Show } from "solid-js";
import { M } from "~/mock";
import { platform } from "~/platform";
import type { GoogleFilesAnswer } from "~/sync/google-files-actions";

const KIND_LABELS: Record<GoogleFileKind, string> = {
  doc: "Doc",
  sheet: "Sheet",
  slides: "Slides",
  file: "File",
};

/** Google gives an https address. Anything else is not drawn as a link. */
const isWebAddress = (url: string): boolean => /^https:\/\//i.test(url);

/** When a file last changed, in words, or nothing when Google did not say. */
function changedAgo(file: GoogleFile): string {
  const at = file.modifiedAt ? Date.parse(file.modifiedAt) : Number.NaN;
  return Number.isNaN(at) ? "" : M.rel(at).toLowerCase();
}

/** The link that opens a file in Google, in the person's own browser. */
function OpenLink(props: { file: GoogleFile }) {
  const open = (event: MouseEvent): void => {
    // The desktop and phone shells have no new tabs, so the system browser opens the address.
    if (!platform().native) return;
    event.preventDefault();
    void platform().openExternal(props.file.url);
  };
  return (
    <a
      href={props.file.url}
      target="_blank"
      rel="noopener noreferrer"
      aria-label={`Open ${props.file.name}`}
      class="flex-none text-small underline"
      onClick={open}
    >
      Open
    </a>
  );
}

/** The rows of the files, and the folder they are kept in. */
function FileRows(props: { files: GoogleFiles; showKind: boolean }) {
  return (
    <Show
      when={props.files.files.length > 0}
      fallback={
        <span class="text-small leading-4.5 text-secondary">
          Marshal has not made any files here yet.
        </span>
      }
    >
      <ul class="m-0 p-0 list-none flex flex-col gap-1.5">
        <For each={props.files.files}>
          {(file) => (
            <li class="flex items-center gap-2 text-small">
              <span class="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">
                {file.name}
              </span>
              <Show when={props.showKind}>
                <span class="flex-none text-caption text-muted">{KIND_LABELS[file.kind]}</span>
              </Show>
              <span class="flex-none text-caption text-muted">{changedAgo(file)}</span>
              <Show when={isWebAddress(file.url)}>
                <OpenLink file={file} />
              </Show>
            </li>
          )}
        </For>
      </ul>
      <span class="text-caption leading-4 text-secondary">
        Saved in the folder “{props.files.folder}” in your Drive.
      </span>
    </Show>
  );
}

/**
 * The files Marshal made in Google, newest first, each with a link that opens it. It asks only once
 * the connection is made, and shows the daemon's sentence when it cannot list them.
 */
export function GoogleFilesList(props: {
  name: string;
  /** Which kind of file to list. Every kind Marshal makes when it is left out. */
  kind?: GoogleFileKind;
  connected: boolean;
  /** Reads the files. The store's own call by default; a test gives another. */
  load?: (kind?: GoogleFileKind) => Promise<GoogleFilesAnswer>;
  /** Called with the Drive folder's name each time the files are read. */
  onFolder?: (folder: string) => void;
}) {
  const [loaded, { refetch }] = createResource<GoogleFilesAnswer | null, boolean>(
    () => props.connected,
    async (connected) =>
      connected ? (props.load ?? ((kind) => M.googleFiles(kind)))(props.kind) : null,
  );
  createEffect(() => {
    const answer = loaded();
    if (answer && "files" in answer) props.onFolder?.(answer.files.folder);
  });
  return (
    <div class="flex flex-col gap-2">
      <span class="text-small font-semibold">Files Marshal made</span>
      <Show
        when={props.connected}
        fallback={
          <span class="text-small leading-4.5 text-secondary">
            Connect {props.name} first. The files Marshal makes show up here.
          </span>
        }
      >
        <Show when={loaded.loading}>
          <span class="text-small text-secondary">Reading your files…</span>
        </Show>
        <Show when={loaded()}>
          {(answer) => (
            <Show
              when={"files" in answer() ? (answer() as { files: GoogleFiles }).files : null}
              fallback={
                <div class="flex flex-wrap items-center gap-2 text-small text-status-danger-text">
                  <span role="alert">{(answer() as { error: string }).error}</span>
                  <Button size={28} onClick={() => void refetch()}>
                    Try again
                  </Button>
                </div>
              }
            >
              {(files) => <FileRows files={files()} showKind={props.kind === undefined} />}
            </Show>
          )}
        </Show>
      </Show>
    </div>
  );
}
