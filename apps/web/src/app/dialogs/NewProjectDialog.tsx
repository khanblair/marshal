import {
  Button,
  Callout,
  Dialog,
  Field,
  Icon,
  Input,
  SegmentedControl,
  type SegmentOption,
} from "@marshal/ui";
import { Show } from "solid-js";
import { M, type NewProjectDraft } from "~/mock";
import { DialogHeader } from "./DialogHeader";
import {
  addDraftProject,
  canAdd,
  detectKind,
  detectMessage,
  followName,
  patchDraft,
  sourceOf,
} from "./new-project";

const SOURCES: readonly SegmentOption<NewProjectDraft["source"]>[] = [
  { value: "folder", label: "Pick a folder", icon: "folder-open" },
  { value: "github", label: "Clone from GitHub", icon: "github" },
];
const CHOSEN_FOLDER = "~/code/billing-service";
const CHOSEN_NAME = "billing-service";

const closeNewProject = (): void => M.set({ newProject: null });

/** The path field and its Choose folder button (a folder picker in the real app). */
function FolderField(props: { draft: NewProjectDraft }) {
  const draft = () => props.draft;
  return (
    <Field
      label="Repository folder"
      hint="Marshal reads this repository and makes worktrees beside it. It never moves or deletes your files."
    >
      <span class="flex gap-2">
        <Input
          mono
          class="flex-1 min-w-0"
          value={draft().path}
          onInput={(e) => {
            const path = e.currentTarget.value;
            patchDraft(draft(), { path, name: followName(draft(), path) });
          }}
          placeholder="~/code/my-repo"
        />
        <Button
          class="flex-none hover:bg-surface!"
          onClick={() =>
            patchDraft(draft(), {
              path: CHOSEN_FOLDER,
              name: draft().nameTouched ? draft().name : CHOSEN_NAME,
            })
          }
        >
          Choose folder
        </Button>
      </span>
    </Field>
  );
}

function GithubField(props: { draft: NewProjectDraft }) {
  return (
    <Field label="Repository URL" hint="Marshal clones it into ~/code using the GitHub App.">
      <Input
        mono
        value={props.draft.url}
        onInput={(e) => {
          const url = e.currentTarget.value;
          patchDraft(props.draft, { url, name: followName(props.draft, url) });
        }}
        placeholder="https://github.com/owner/repo"
      />
    </Field>
  );
}

function NewProjectForm(props: { draft: NewProjectDraft }) {
  const draft = () => props.draft;
  const kind = () => detectKind(sourceOf(draft()));
  return (
    <Dialog
      width={560}
      phone={M.mobile}
      aria-labelledby="np-title"
      onClose={closeNewProject}
      onSubmit={(e) => {
        e.preventDefault();
        addDraftProject(draft());
      }}
    >
      <DialogHeader id="np-title" title="New project" onClose={closeNewProject} />
      <SegmentedControl
        size={30}
        fill
        unselectedTone="primary"
        label="Where the repository comes from"
        options={SOURCES}
        value={draft().source || "folder"}
        onValueChange={(source) => patchDraft(draft(), { source })}
      />
      <Show when={draft().source !== "github"}>
        <FolderField draft={draft()} />
      </Show>
      <Show when={draft().source === "github"}>
        <GithubField draft={draft()} />
      </Show>
      <div class="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3">
        <Field label="Name">
          <Input
            value={draft().name}
            onInput={(e) => patchDraft(draft(), { name: e.currentTarget.value, nameTouched: true })}
          />
        </Field>
        <Field label="Default branch">
          <Input
            mono
            value={draft().branch}
            onInput={(e) => patchDraft(draft(), { branch: e.currentTarget.value })}
          />
        </Field>
      </div>
      <Callout tone="neutral" class="items-center">
        <Icon name={kind() ? "scan-search" : "info"} size={14} />
        <span>{detectMessage(kind(), draft().branch)}</span>
      </Callout>
      <div class="flex justify-end gap-2">
        <Button class="hover:bg-surface!" onClick={closeNewProject}>
          Cancel
        </Button>
        <Button variant="primary" type="submit" class="hover:bg-ink!" disabled={!canAdd(draft())}>
          Add project
        </Button>
      </div>
    </Dialog>
  );
}

/**
 * The New project form: pick a folder or clone from GitHub, and see what Marshal detects.
 * Renders nothing while `M.S.newProject` is empty.
 */
export function NewProjectDialog() {
  return <Show when={M.S.newProject}>{(draft) => <NewProjectForm draft={draft()} />}</Show>;
}
