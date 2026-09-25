import { createMemo, createSignal } from "solid-js";
import { M } from "~/mock";
import { sameJson } from "./json";

export interface ProjectFields {
  name: string;
  branch: string;
  dev: string;
  lockBypass: boolean;
}

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
      // Exactly what the daemon has: a project that has no dev command shows none.
      branch: p?.branch ?? "",
      dev: p?.dev ?? "",
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
    /** Saves to the daemon. The form keeps the person's edits until the daemon has accepted them. */
    async save(): Promise<void> {
      const target = project();
      if (!target || unchanged()) return;
      const next = fields();
      if (!next.name.trim()) {
        M.toast("Project names can't be empty. The old name is kept.");
        return;
      }
      const saved = await M.saveProject(target.id, { ...next, name: next.name.trim() });
      if (saved) setEdited(null);
    },
    remove(): void {
      const id = projectId();
      if (id) M.set({ removeProject: { id, keepBranches: true, keepMemory: true } });
    },
  };
}

export type ProjectDraft = ReturnType<typeof createProjectDraft>;
