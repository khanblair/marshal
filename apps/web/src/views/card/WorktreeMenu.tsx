import { Button, Menu, MenuItem } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import { M } from "~/mock";
import type { CardKey } from "~/mock/card-key";
import { openCardWorktree } from "~/sync/integration-flow";

type Opener = "finder" | "editor";

export interface WorktreeMenuProps {
  /** The card's key, which the sync functions take. */
  id: CardKey;
  /** The worktree folder on the daemon's machine. */
  path: string;
}

/** "Show worktree": copy the folder's path, reveal it in Finder, or open it in the editor. */
export function WorktreeMenu(props: WorktreeMenuProps) {
  const [open, setOpen] = createSignal(false);
  let trigger: HTMLButtonElement | undefined;
  const close = () => setOpen(false);
  const copy = () => {
    close();
    navigator.clipboard?.writeText(props.path).catch(() => undefined);
    M.toast("Worktree path copied");
  };
  const openIn = (opener: Opener) => () => {
    close();
    void openCardWorktree(props.id, opener);
  };
  return (
    <div class="relative">
      <Button
        ref={(el) => {
          trigger = el;
        }}
        size={28}
        icon="folder-git-2"
        aria-haspopup="menu"
        aria-expanded={open()}
        title={props.path}
        onClick={() => setOpen(!open())}
      >
        Show worktree
      </Button>
      <Show when={open()}>
        <Menu
          sheet={M.mobile}
          onClose={close}
          trigger={() => trigger}
          class={M.mobile ? undefined : "absolute top-[calc(100%+4px)] left-0 w-52 z-menu"}
        >
          <MenuItem icon="copy" onClick={copy}>
            Copy path
          </MenuItem>
          <MenuItem icon="folder-open" onClick={openIn("finder")}>
            Reveal in Finder
          </MenuItem>
          <MenuItem icon="square-terminal" onClick={openIn("editor")}>
            Open in editor
          </MenuItem>
        </Menu>
      </Show>
    </div>
  );
}
