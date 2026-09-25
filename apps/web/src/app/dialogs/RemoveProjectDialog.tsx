import { Button, Checkbox, Dialog, Icon } from "@marshal/ui";
import { createMemo, For, Show } from "solid-js";
import { M, type RemoveProjectDraft } from "~/mock";
import { type RemoveModel, removeModel } from "./remove-project";

const ICON_PX = 14;

const closeRemove = (): void => M.set({ removeProject: null });

function confirmRemove(draft: RemoveProjectDraft): void {
  const { id } = draft;
  M.set({ removeProject: null });
  M.removeProject(id);
}

/** The unmerged-work box, with its keep-branches option. */
function UnmergedWork(props: { draft: RemoveProjectDraft; model: RemoveModel }) {
  return (
    <div class="flex flex-col gap-2 p-3 rounded-md bg-status-needs-you-subtle text-status-needs-you-text">
      <span class="flex items-center gap-1.5 font-semibold">
        <Icon name="git-branch" size={ICON_PX} />
        {props.model.unmergedLabel}
      </span>
      <label class="flex items-center gap-2 cursor-pointer text-primary">
        <Checkbox
          checked={props.draft.keepBranches}
          onChange={() => {
            props.draft.keepBranches = !props.draft.keepBranches;
          }}
        />
        Keep their branches in the repository
      </label>
    </div>
  );
}

function RemoveForm(props: { draft: RemoveProjectDraft; model: RemoveModel }) {
  return (
    <Dialog
      role="alertdialog"
      phone={M.mobile}
      width={520}
      aria-labelledby="rp-title"
      onClose={closeRemove}
    >
      <h2 id="rp-title" class="m-0 text-title leading-6 font-semibold">
        Remove {props.model.name}
      </h2>
      <p class="m-0 font-semibold">
        The repository on disk is never deleted. Marshal only stops managing it.
      </p>
      <div class="flex flex-col gap-1.5">
        <span class="font-medium">What happens</span>
        <For each={props.model.effects}>
          {(effect) => (
            <span class="flex gap-2 text-secondary">
              <span class="inline-flex mt-0.5">
                <Icon name={effect.icon} size={ICON_PX} />
              </span>
              <span>{effect.text}</span>
            </span>
          )}
        </For>
      </div>
      <Show when={props.model.unmerged > 0}>
        <UnmergedWork draft={props.draft} model={props.model} />
      </Show>
      <label class="flex items-start gap-2 cursor-pointer">
        <Checkbox
          align="start"
          checked={props.draft.keepMemory}
          onChange={() => {
            props.draft.keepMemory = !props.draft.keepMemory;
          }}
        />
        <span class="flex flex-col">
          Keep the project's memory folder
          <span class="font-mono text-caption text-secondary">{props.model.memoryPath}</span>
        </span>
      </label>
      <div class="flex justify-end flex-wrap gap-2">
        <Button class="hover:bg-surface!" onClick={closeRemove}>
          Cancel
        </Button>
        <Button variant="destructive" onClick={() => confirmRemove(props.draft)}>
          Remove project
        </Button>
      </div>
    </Dialog>
  );
}

/**
 * The Remove project confirmation: what stops, what stays, and the keep-branches and
 * keep-memory options. Renders nothing while `M.S.removeProject` is empty or names a project
 * that no longer exists.
 */
export function RemoveProjectDialog() {
  const active = createMemo(() => {
    const draft = M.S.removeProject;
    const model = draft ? removeModel(M, draft.id) : null;
    return draft && model ? { draft, model } : null;
  });
  return <Show when={active()}>{(a) => <RemoveForm draft={a().draft} model={a().model} />}</Show>;
}
