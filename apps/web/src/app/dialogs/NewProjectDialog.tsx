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
import { addDraftProject, canAdd, followName, IDLE_MESSAGE, patchDraft } from "./new-project";

/**
 * Under this width the source options drop their icons: two labels and two icons do not fit side by
 * side at 320 px, and "Clone from GitHub" would wrap onto a second line.
 */
const ICONS_MIN_WIDTH_PX = 360;

const SOURCES: readonly SegmentOption<NewProjectDraft["source"]>[] = [
  { value: "folder", label: "Pick a folder", icon: "folder-open" },
  { value: "github", label: "Clone from GitHub", icon: "github" },
];
const PLAIN_SOURCES: readonly SegmentOption<NewProjectDraft["source"]>[] = SOURCES.map(
  ({ value, label }) => ({ value, label }),
);

const closeNewProject = (): void => M.set({ newProject: null });

/**
 * The path field. A web page cannot open a folder picker, so the path is typed (the daemon expands a
 * leading `~`). The desktop app will add a Choose folder button.
 */
function FolderField(props: { draft: NewProjectDraft }) {
  const draft = () => props.draft;
  return (
    <Field
      label="Repository folder"
      hint="Marshal reads this repository and makes worktrees beside it. It never moves or deletes your files."
    >
      <Input
        mono
        value={draft().path}
        onInput={(e) => {
          const path = e.currentTarget.value;
          patchDraft(draft(), { path, name: followName(draft(), path) });
        }}
        placeholder="~/code/my-repo"
      />
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

/** The line under the fields: what Marshal does, or the daemon's sentence when it refused. */
function StatusLine(props: { draft: NewProjectDraft }) {
  return (
    <Show
      when={props.draft.error}
      fallback={
        <Callout tone="neutral" class="items-center">
          <Icon name="info" size={14} />
          <span>{IDLE_MESSAGE}</span>
        </Callout>
      }
    >
      {(message) => (
        <Callout role="alert" class="items-center">
          <Icon name="triangle-alert" size={14} />
          <span>{message()}</span>
        </Callout>
      )}
    </Show>
  );
}

function NewProjectForm(props: { draft: NewProjectDraft }) {
  const draft = () => props.draft;
  return (
    <Dialog
      width={560}
      phone={M.mobile}
      aria-labelledby="np-title"
      onClose={closeNewProject}
      onSubmit={(e) => {
        e.preventDefault();
        void addDraftProject(draft());
      }}
    >
      <DialogHeader id="np-title" title="New project" onClose={closeNewProject} />
      <SegmentedControl
        size={30}
        fill
        unselectedTone="primary"
        label="Where the repository comes from"
        options={M.S.vw < ICONS_MIN_WIDTH_PX ? PLAIN_SOURCES : SOURCES}
        value={draft().source || "folder"}
        onValueChange={(source) => patchDraft(draft(), { source, error: "" })}
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
        <Show when={draft().source === "github"}>
          <Field label="Branch">
            <Input
              mono
              value={draft().branch}
              placeholder="Repository default"
              onInput={(e) => patchDraft(draft(), { branch: e.currentTarget.value })}
            />
          </Field>
        </Show>
      </div>
      <StatusLine draft={draft()} />
      <div class="flex justify-end gap-2">
        <Button class="hover:bg-surface!" onClick={closeNewProject}>
          Cancel
        </Button>
        <Button
          variant="primary"
          type="submit"
          class="hover:bg-ink!"
          disabled={!canAdd(draft()) || !!draft().busy}
        >
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
