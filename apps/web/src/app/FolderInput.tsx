import type { FolderListing } from "@marshal/protocol";
import { Button, Icon, Input } from "@marshal/ui";
import { createSignal, For, Show } from "solid-js";
import { isDesktop, pickFolder } from "~/data/desktop";
import { M } from "~/mock";

export interface FolderInputProps {
  value: string;
  onChange: (path: string) => void;
  placeholder?: string;
}

/**
 * A repository folder: a field for a typed path and a Browse button. In the desktop app Browse opens
 * the system's own folder dialog. Anywhere else it opens a browser of the folders of the computer
 * Marshal runs on, because a web page can never read the path of a folder it picked itself. The
 * browser lists folders only, marks the ones that hold a Git repository, and picks the one that is
 * open with "Use this folder".
 */
export function FolderInput(props: FolderInputProps) {
  const [listing, setListing] = createSignal<FolderListing | null>(null);
  const [loading, setLoading] = createSignal(false);

  const open = async (path?: string): Promise<void> => {
    setLoading(true);
    const found = await M.browseFolders(path);
    setLoading(false);
    if (found) setListing(found);
  };
  const browse = async (): Promise<void> => {
    if (isDesktop()) {
      const chosen = await pickFolder();
      if (chosen) props.onChange(chosen);
      return;
    }
    // It starts where the typed path points, or at home when that is not a folder that opens.
    await open(props.value.trim() || undefined);
    if (!listing() && props.value.trim()) await open();
  };
  const choose = (path: string): void => {
    props.onChange(path);
    setListing(null);
  };

  return (
    <div class="flex flex-col gap-2">
      <div class="flex gap-2">
        <Input
          mono
          class="flex-1 min-w-0"
          value={props.value}
          onInput={(e) => props.onChange(e.currentTarget.value)}
          placeholder={props.placeholder ?? "~/code/my-repo"}
        />
        <Button type="button" icon="folder-open" disabled={loading()} onClick={() => void browse()}>
          Browse…
        </Button>
      </div>
      <Show when={listing()}>
        {(current) => (
          <fieldset class="m-0 flex min-w-0 flex-col rounded-md border border-border bg-surface p-0">
            <legend class="sr-only">Choose a folder</legend>
            <div class="flex items-center gap-2 border-b border-border px-2.5 py-2">
              <Button
                type="button"
                size={28}
                icon="arrow-up"
                aria-label="Up one folder"
                disabled={!current().parent || loading()}
                onClick={() => void open(current().parent)}
              />
              <code class="flex-1 min-w-0 truncate font-mono text-small" title={current().path}>
                {current().path}
              </code>
              <Show when={current().isGitRepo}>
                <span class="inline-flex items-center gap-1 text-small font-semibold text-status-working-text">
                  <Icon name="check" size={14} />
                  Git repository
                </span>
              </Show>
            </div>
            <ul class="m-0 max-h-60 overflow-auto p-0 list-none">
              <For
                each={current().folders}
                fallback={<li class="px-3 py-3 text-small text-secondary">No folders in here.</li>}
              >
                {(folder) => (
                  <li>
                    <button
                      type="button"
                      class="flex w-full items-center gap-2.5 border-none bg-transparent px-3 py-2 text-left hover:bg-surface-hover"
                      onClick={() => void open(folder.path)}
                    >
                      <Icon name={folder.isGitRepo ? "folder-git-2" : "folder-open"} size={16} />
                      <span class="flex-1 min-w-0 truncate">{folder.name}</span>
                      <Show when={folder.isGitRepo}>
                        <span class="text-small text-secondary">Git</span>
                      </Show>
                      <Icon name="chevron-right" size={14} />
                    </button>
                  </li>
                )}
              </For>
            </ul>
            <Show when={current().truncated}>
              <span class="border-t border-border px-3 py-2 text-small text-secondary">
                This folder has more folders than can be shown. Type the path instead.
              </span>
            </Show>
            <div class="flex items-center gap-2 border-t border-border px-2.5 py-2">
              <span class="flex-1" />
              <Button type="button" onClick={() => setListing(null)}>
                Cancel
              </Button>
              <Button type="button" variant="primary" onClick={() => choose(current().path)}>
                Use this folder
              </Button>
            </div>
          </fieldset>
        )}
      </Show>
    </div>
  );
}
