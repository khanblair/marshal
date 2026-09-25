import { Button, CiStatus, ciAppearance, Icon } from "@marshal/ui";
import { createMemo, For, Index, Show } from "solid-js";
import { M } from "~/mock";
import { CiNotConnected } from "./CiNotConnected";
import { type CiRun, ciInfo, hasCi, type ProjectWithCi, runAgo, runsOf } from "./ci-rows";

function RunRow(props: { run: CiRun }) {
  const look = () => ciAppearance(props.run.state);
  return (
    <button
      type="button"
      onClick={() => props.run.open()}
      class="flex items-center gap-2.5 min-h-10 py-1.5 px-0 border-x-0 border-t-0 border-b border-border bg-transparent text-left hover:bg-surface-hover"
    >
      <Icon name={look().icon} size={14} class={look().color} />
      <span class="flex-1 min-w-0 flex flex-col">
        <span class="font-mono text-small overflow-hidden text-ellipsis whitespace-nowrap">
          {props.run.name}
        </span>
        <span class="text-caption leading-4 text-secondary">{props.run.where}</span>
      </span>
      <span class={`text-small font-semibold ${look().color}`}>{M.CI[props.run.state].label}</span>
      <span class="w-21 text-right text-caption text-muted">{runAgo(props.run.minutesAgo)}</span>
    </button>
  );
}

function ProjectRuns(props: { project: ProjectWithCi }) {
  const runs = createMemo(() => runsOf(props.project));
  return (
    <section class="flex flex-col pb-2">
      <div class="flex flex-wrap items-center gap-2 py-2.5 border-b border-border">
        <h3 class="m-0 flex-1 text-subtitle leading-5.5 font-semibold">{props.project.name}</h3>
        <CiStatus status={props.project.ci} class="text-small">
          Main <span>{ciInfo(props.project.ci).label.toLowerCase()}</span>
        </CiStatus>
        <Button
          class="px-2.5! text-small hover:bg-surface!"
          onClick={() => M.go("project", props.project.id, "board")}
        >
          Open board
        </Button>
      </div>
      <Index each={runs()}>{(run) => <RunRow run={run()} />}</Index>
    </section>
  );
}

/**
 * The CI health page: the main branch of every project that has CI data, with its workflow runs and
 * its cards' runs. A project with no CI data is left out, and when none has any the page says GitHub
 * is not connected instead of drawing an empty list.
 */
export function CiPage(props: { projectId: string }) {
  const projects = createMemo(() =>
    M.S.projects.filter(hasCi).filter((p) => props.projectId === "all" || p.id === props.projectId),
  );
  return (
    <Show when={projects().length > 0} fallback={<CiNotConnected />}>
      <For each={projects()}>{(project) => <ProjectRuns project={project} />}</For>
    </Show>
  );
}
