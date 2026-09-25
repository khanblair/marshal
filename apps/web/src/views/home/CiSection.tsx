import { CiStatus, ShowMoreFooter } from "@marshal/ui";
import { createMemo, For, Show } from "solid-js";
import { M } from "~/mock";
import { CiNotConnected } from "./CiNotConnected";
import { ciInfo, hasCi, homeCiAgo, type ProjectWithCi } from "./ci-rows";
import { viewAllCi } from "./home-actions";
import { createShowAll } from "./use-show-all";

function CiRow(props: { project: ProjectWithCi }) {
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

/** Home's "CI health": the state of each project's main branch, for the projects that have CI data. */
export function CiSection() {
  const withCi = createMemo(() => M.S.projects.filter(hasCi));
  const rows = createShowAll(withCi);
  return (
    <section aria-labelledby="h-ci" class="flex flex-col">
      <h2 id="h-ci" class="m-0 mb-2 text-subtitle leading-5.5 font-semibold">
        CI health
      </h2>
      <Show when={withCi().length > 0} fallback={<CiNotConnected />}>
        <For each={rows.visible()}>{(project) => <CiRow project={project} />}</For>
        <ShowMoreFooter
          total={withCi().length}
          expanded={rows.expanded()}
          onToggle={rows.toggle}
          viewAllLabel="View all CI runs"
          onViewAll={viewAllCi}
        />
      </Show>
    </section>
  );
}
