import { CiStatus, CountBubble, cx, Icon, IconButton, NeedsBadge } from "@marshal/ui";
import { createMemo, Show } from "solid-js";
import { M, type Project } from "~/mock";
import { ProjectMenu } from "./ProjectMenu";
import { ProjectRenameInput } from "./ProjectRenameInput";
import { toggleMenu } from "./shell-actions";
import { isProject, sidebarOpen } from "./shell-layout";
import { goProject } from "./sidebar-actions";

export interface ProjectRowProps {
  project: Project;
}

/** One project in the sidebar: name, needs-you count, CI state, awake agents, and a menu. */
export function ProjectRow(props: ProjectRowProps) {
  const id = () => props.project.id;
  const needs = createMemo(() => M.needs(id()).length);
  const awake = createMemo(() => M.awake(id()).length);
  const ci = () => M.CI[props.project.ci] || M.CI.queued;
  const selected = () => isProject() && M.S.route.pid === id();
  const menuName = () => `proj:${id()}`;
  const menuOpen = () => M.S.menu === menuName();
  const tip = () =>
    `${props.project.name}: ${needs()} need you, main CI ${ci().label.toLowerCase()}, ${awake()} awake agents`;

  return (
    // biome-ignore lint/a11y/useSemanticElements: the design's row is a div with a listitem role, so it keeps the exact layout
    <div
      role="listitem"
      class={cx(
        "relative flex items-center rounded-sm",
        selected() ? "bg-surface-selected" : "bg-transparent",
      )}
    >
      <Show when={selected()}>
        <span class="absolute -left-2 top-2 bottom-2 w-0.5 rounded-2xs bg-ink" />
      </Show>
      <Show when={M.S.renaming !== id()} fallback={<ProjectRenameInput project={props.project} />}>
        <button
          type="button"
          onClick={() => goProject(id())}
          title={tip()}
          aria-label={tip()}
          class={cx(
            "flex-1 min-w-0 flex items-center gap-2 min-h-9 py-1.5 border-none rounded-sm bg-transparent text-primary text-left hover:bg-surface-hover",
            sidebarOpen() ? "px-2 justify-start" : "px-0 justify-center",
          )}
        >
          <span class="relative size-5 flex-none inline-flex items-center justify-center rounded-sm border border-border-strong text-caption font-bold text-secondary">
            {props.project.name[0] ? props.project.name[0].toUpperCase() : "?"}
            <Show when={needs() > 0 && !sidebarOpen()}>
              <CountBubble tone="needs-you" class="absolute -right-2 -top-2">
                {needs()}
              </CountBubble>
            </Show>
          </span>
          <Show when={sidebarOpen()}>
            <span class="flex-1 min-w-0 truncate font-medium">{props.project.name}</span>
            <span class="flex items-center gap-1.5 flex-none">
              <Show when={needs() > 0}>
                <NeedsBadge count={needs()} class="px-1.25!" />
              </Show>
              <CiStatus status={props.project.ci} />
              <span class="inline-flex items-center gap-0.5 text-caption text-secondary">
                <Icon name="zap" size={12} />
                {awake()}
              </span>
            </span>
          </Show>
        </button>
        <Show when={sidebarOpen()}>
          <IconButton
            label={`More actions for ${props.project.name}`}
            icon="ellipsis-vertical"
            iconSize={14}
            tone="muted"
            class="w-6! mr-0.5"
            aria-expanded={menuOpen()}
            onClick={(e) => {
              e.stopPropagation();
              toggleMenu(menuName());
            }}
          />
        </Show>
      </Show>
      <Show when={menuOpen()}>
        <ProjectMenu id={id()} />
      </Show>
    </div>
  );
}
