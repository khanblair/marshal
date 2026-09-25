import { cx, Icon } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M } from "~/mock";
import { ProjectRow } from "./ProjectRow";
import { sidebarOpen } from "./shell-layout";
import { openNewProject } from "./sidebar-actions";

/** The Projects section of the sidebar: one row per project and the New project button. */
export function ProjectList() {
  return (
    <div class="flex-1 min-h-0 overflow-auto flex flex-col">
      <Show when={sidebarOpen()}>
        <span
          id="projects-heading"
          class="pt-3 px-4 pb-1 text-small leading-4.5 text-secondary font-medium"
        >
          Projects
        </span>
      </Show>
      {/* biome-ignore lint/a11y/useSemanticElements: the design's list is a div with a list role, so it keeps the exact layout */}
      <div
        role="list"
        aria-labelledby="projects-heading"
        data-tour="projects"
        class="py-1 px-2 flex flex-col gap-0.5"
      >
        <For each={M.S.projects}>{(project) => <ProjectRow project={project} />}</For>
        <button
          type="button"
          data-tour="new-project"
          onClick={openNewProject}
          title="New project"
          aria-label="New project"
          class={cx(
            "flex items-center gap-2 min-h-8 border-none rounded-sm bg-transparent text-secondary font-medium text-left hover:bg-surface-hover hover:text-primary",
            sidebarOpen() ? "px-2 justify-start" : "px-0 justify-center",
          )}
        >
          <Icon name="folder-plus" size={16} />
          <Show when={sidebarOpen()}>
            <span>New project</span>
          </Show>
        </button>
      </div>
    </div>
  );
}
