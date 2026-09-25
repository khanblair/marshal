import { batch, createMemo, createSignal } from "solid-js";
import { M } from "~/mock";
import { sameJson } from "./json";

export interface ProjectFields {
  name: string;
  branch: string;
  dev: string;
  lockBypass: boolean;
}

const DEFAULT_BRANCH = "main";
const DEFAULT_DEV_COMMAND = "pnpm dev";
/** The design's api project runs no dev command by default. */
const NO_DEV_PROJECT_ID = "api";

interface Edited {
  pid: string | null | undefined;
  fields: ProjectFields;
}

/** The project shown: the one picked here, else the open project, else the first. */
function shownProjectId(): string | null | undefined {
  const { settingsPid, route, projects } = M.S;
  if (settingsPid && M.proj(settingsPid)) return settingsPid;
  return M.proj(route.pid) ? route.pid : projects[0]?.id;
}

/** The project form's edits. An edit belongs to the project it was made on. */
export function createProjectDraft() {
  const [edited, setEdited] = createSignal<Edited | null>(null);
  const projectId = createMemo(shownProjectId);
  const project = createMemo(() => M.proj(projectId()));
  const saved = createMemo<ProjectFields>(() => {
    const p = project();
    return {
      name: p?.name ?? "",
      branch: p?.branch || DEFAULT_BRANCH,
      dev: p?.dev || (p?.id === NO_DEV_PROJECT_ID ? "" : DEFAULT_DEV_COMMAND),
      lockBypass: !!p?.lockBypass,
    };
  });
  const fields = () => {
    const current = edited();
    return current && current.pid === projectId() ? current.fields : saved();
  };
  const unchanged = () => sameJson(fields(), saved());
  return {
    projectId,
    project,
    fields,
    unchanged,
    pick(id: string): void {
      M.S.settingsPid = id;
      setEdited(null);
    },
    edit<K extends keyof ProjectFields>(key: K, value: ProjectFields[K]): void {
      setEdited({ pid: projectId(), fields: { ...fields(), [key]: value } });
    },
    save(): void {
      const target = project();
      if (!target || unchanged()) return;
      const next = fields();
      if (!next.name.trim()) {
        M.toast("Project names can't be empty. The old name is kept.");
        return;
      }
      batch(() => {
        Object.assign(target, {
          name: next.name.trim(),
          branch: next.branch,
          dev: next.dev,
          lockBypass: next.lockBypass,
        });
        setEdited(null);
        M.toast("Project saved");
      });
    },
    remove(): void {
      const id = projectId();
      if (id) M.set({ removeProject: { id, keepBranches: true, keepMemory: true } });
    },
  };
}

export type ProjectDraft = ReturnType<typeof createProjectDraft>;
