import { CiStatus, ShowMoreFooter } from "@marshal/ui";
import { For } from "solid-js";
import { M, type Project } from "~/mock";
import { ciInfo, homeCiAgo } from "./ci-rows";
import { viewAllCi } from "./home-actions";
import { createShowAll } from "./use-show-all";

function CiRow(props: { project: Project }) {
  return (
    <button
      type="button"
      onClick={() => M.go("project", props.project.id)}
      class="flex items-center gap-2 py-2 px-0 border-x-0 border-b-0 border-t border-border bg-transparent text-left hover:bg-surface-hover"
    >
      <span class="flex-1 font-semibold">{props.project.name}</span>
      <CiStatus status={props.project.ci} class="text-small">
        Main <span>{ciInfo(props.project.ci).label.toLowerCase()}</span>
      </CiStatus>
      <span class="text-caption text-muted">{homeCiAgo(props.project.ciAgo)}</span>
    </button>
  );
}

/** Home's "CI health": the state of each project's main branch. */
export function CiSection() {
  const rows = createShowAll(() => M.S.projects);
  return (
    <section aria-labelledby="h-ci" class="flex flex-col">
      <h2 id="h-ci" class="m-0 mb-2 text-subtitle leading-5.5 font-semibold">
        CI health
      </h2>
      <For each={rows.visible()}>{(project) => <CiRow project={project} />}</For>
      <ShowMoreFooter
        total={M.S.projects.length}
        expanded={rows.expanded()}
        onToggle={rows.toggle}
        viewAllLabel="View all CI runs"
        onViewAll={viewAllCi}
      />
    </section>
  );
}
