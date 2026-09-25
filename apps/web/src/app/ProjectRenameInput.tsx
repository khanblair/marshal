import { Input } from "@marshal/ui";
import { batch, untrack } from "solid-js";
import { M, type Project } from "~/mock";

export interface ProjectRenameInputProps {
  project: Pick<Project, "id" | "name">;
}

/** Wait before focusing, so the field exists in the page when it takes focus. */
const FOCUS_DELAY_MS = 0;

/**
 * The project name field that replaces a sidebar row while it is renamed. Enter and blur
 * save, Escape cancels. The field is not controlled: it starts with the current name.
 */
export function ProjectRenameInput(props: ProjectRenameInputProps) {
  /** Ends the rename once; blur after Enter or Escape finds it already ended. */
  const finish = (save: string | null) => {
    if (M.S.renaming !== props.project.id) return;
    batch(() => {
      M.S.renaming = null;
      if (save !== null && save !== props.project.name) M.renameProject(props.project.id, save);
    });
  };

  const focusAndSelect = (el: HTMLInputElement) => {
    setTimeout(() => {
      el.focus();
      el.select();
    }, FOCUS_DELAY_MS);
  };

  return (
    <Input
      ref={focusAndSelect}
      value={untrack(() => props.project.name)}
      aria-label="Project name"
      class="flex-1 min-w-0 m-0.5 px-2! font-medium"
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          finish(e.currentTarget.value);
        }
        if (e.key === "Escape") {
          e.preventDefault();
          e.stopPropagation();
          finish(null);
        }
      }}
      onBlur={(e) => finish(e.currentTarget.value)}
    />
  );
}
