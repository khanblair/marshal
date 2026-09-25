import { batch } from "solid-js";
import { M, type NewProjectDraft } from "~/mock";

/** What the New project dialog says it found: a monorepo, a Go project, or a TypeScript one. */
type Detected = "mono" | "Go" | "TypeScript";

/** The path or URL the user typed, whichever source is selected. */
export const sourceOf = (draft: NewProjectDraft): string =>
  draft.source === "github" ? draft.url || "" : draft.path || "";

/** The prototype's guess from the text of the path or URL. Nothing typed, nothing detected. */
export function detectKind(source: string): Detected | null {
  if (!source) return null;
  if (/mono|apps|workspace/i.test(source)) return "mono";
  return /go|api|service/i.test(source) ? "Go" : "TypeScript";
}

/** The last path segment, without `.git`: `git@host:org/repo.git` gives `repo`. */
export const autoName = (source: string): string =>
  source
    .replace(/\.git$/, "")
    .split(/[/:]/)
    .filter(Boolean)
    .pop() || "";

const MONO_MESSAGE =
  "Detected a monorepo: pnpm workspaces with 3 packages. It gets one board with package swimlanes.";
const IDLE_MESSAGE =
  "Choose a folder or paste a URL. Marshal detects the language and monorepo tools.";

/** The status line under the fields. */
export function detectMessage(kind: Detected | null, branch: string): string {
  if (!kind) return IDLE_MESSAGE;
  if (kind === "mono") return MONO_MESSAGE;
  return `Detected a ${kind} project on branch ${branch || "main"}.`;
}

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

/** Adds the project, opens its board, and confirms with a toast. */
export function addDraftProject(draft: NewProjectDraft): void {
  const source = sourceOf(draft);
  if (!canAdd(draft)) return;
  const kind = detectKind(source);
  const id = M.addProject({
    name: draft.name.trim(),
    path: draft.source === "github" ? `~/code/${autoName(source)}` : source,
    branch: draft.branch,
    mono: kind === "mono",
    lang: kind === "Go" ? "Go" : "TypeScript",
  });
  batch(() => {
    M.set({ newProject: null });
    M.go("project", id, "board");
    M.toast("Project added");
  });
}
