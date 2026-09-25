import type { CreateProjectRequest } from "@marshal/protocol";
import { batch } from "solid-js";
import { M, type NewProjectDraft } from "~/mock";

/** The path or URL the user typed, whichever source is selected. */
export const sourceOf = (draft: NewProjectDraft): string =>
  draft.source === "github" ? draft.url || "" : draft.path || "";

/** The last path segment, without `.git`: `git@host:org/repo.git` gives `repo`. */
export const autoName = (source: string): string =>
  source
    .replace(/\.git$/, "")
    .split(/[/:]/)
    .filter(Boolean)
    .pop() || "";

/**
 * What the line under the fields says before the daemon has answered. The daemon reads the language
 * and the monorepo tools from the repository when it adds the project, so nothing is guessed here
 * from the text typed: what it found shows on the project's board.
 */
export const IDLE_MESSAGE =
  "Choose a folder or paste a URL. Marshal detects the language and monorepo tools.";

/** A project can be added once it has a path or URL and a name. */
export const canAdd = (draft: NewProjectDraft): boolean =>
  !!sourceOf(draft).trim() && !!(draft.name || "").trim();

/** Merges changes into the draft, as one update. */
export function patchDraft(draft: NewProjectDraft, changes: Partial<NewProjectDraft>): void {
  batch(() => Object.assign(draft, changes));
}

/** The name field follows the path or URL until the user types in it. */
export const followName = (draft: NewProjectDraft, from: string): string =>
  draft.nameTouched ? draft.name : autoName(from);

/**
 * The daemon's request for the draft. A folder is sent as typed (the daemon expands a leading `~`).
 * A clone goes into `~/code/<repository name>`, which is where the dialog says it will go, and
 * carries the branch only when one was typed.
 */
export function requestOf(draft: NewProjectDraft): CreateProjectRequest {
  const name = draft.name.trim();
  if (draft.source !== "github") return { source: "folder", path: draft.path.trim(), name };
  const url = draft.url.trim();
  const branch = draft.branch.trim();
  return {
    source: "clone",
    url,
    dest: `~/code/${autoName(url)}`,
    name,
    ...(branch ? { branch } : {}),
  };
}

/**
 * Adds the project on the daemon. On success the dialog closes, the new board opens, and a toast
 * confirms. On a refusal the dialog stays open with its fields as they were, and the daemon's
 * sentence shows under them. A second press while the first is still running does nothing.
 */
export async function addDraftProject(draft: NewProjectDraft): Promise<void> {
  if (draft.busy || !canAdd(draft)) return;
  patchDraft(draft, { busy: true, error: "" });
  const result = await M.addProject(requestOf(draft));
  if ("error" in result) {
    patchDraft(draft, { busy: false, error: result.error });
    return;
  }
  batch(() => {
    patchDraft(draft, { busy: false });
    M.set({ newProject: null });
    M.go("project", result.id, "board");
    M.toast("Project added");
  });
}
