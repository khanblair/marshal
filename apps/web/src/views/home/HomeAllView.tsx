import { Button, Select } from "@marshal/ui";
import { createSignal, Match, Switch } from "solid-js";
import { M } from "~/mock";
import { ActivityPage } from "./ActivityPage";
import { CiPage } from "./CiPage";
import { pagePadding } from "./home-layout";

/**
 * The full page behind Home's "View all" links: every activity entry with kind
 * filters, or every project's CI runs. `M.S.allKind` picks which one, and a
 * project select narrows both.
 */
export function HomeAllView() {
  const [kind, setKind] = createSignal("all");
  const [projectId, setProjectId] = createSignal("all");
  const isCi = () => M.S.allKind === "ci";
  const projectOptions = () => [
    { value: "all", label: "All projects" },
    ...M.S.projects.map((p) => ({ value: p.id, label: p.name })),
  ];
  return (
    <div class="absolute inset-0 overflow-y-auto overflow-x-hidden">
      <div class={`max-w-[880px] mx-auto flex flex-col gap-4 ${pagePadding()}`}>
        <div class="flex flex-wrap items-center gap-3">
          <Button
            variant="quiet"
            icon="chevron-left"
            class="pl-1.5! pr-2.5!"
            onClick={() => M.go("home")}
          >
            Back to Home
          </Button>
          <span class="flex-1" />
          {/* biome-ignore lint/a11y/noLabelWithoutControl: the Select component renders the select inside this label */}
          <label class="flex items-center gap-2 text-small text-secondary">
            <span>Project</span>
            <Select
              options={projectOptions()}
              value={projectId()}
              onChange={(event) => setProjectId(event.currentTarget.value)}
            />
          </label>
        </div>
        <h2 class="m-0 text-view-title leading-7 font-bold">
          {isCi() ? "CI health" : "Recent activity"}
        </h2>
        <Switch>
          <Match when={isCi()}>
            <CiPage projectId={projectId()} />
          </Match>
          <Match when={!isCi()}>
            <ActivityPage kind={kind()} onKindChange={setKind} projectId={projectId()} />
          </Match>
        </Switch>
      </div>
    </div>
  );
}
