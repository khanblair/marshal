import type { Project } from "@marshal/protocol";
import { toMillis } from "./time";

/**
 * A project as the daemon knows it, in the words the screens use for the same things.
 *
 * It is not the mock's `Project`: the daemon has no CI runs, monthly cost, or CI age yet (they come
 * in later phases), so `ci`, `ciAgo`, `monthBase`, and `runs` are missing here. The screens
 * tolerate that in the change that switches them to the daemon.
 */
export interface DaemonProject {
  id: string;
  name: string;
  /** The language the daemon found, such as "TypeScript", or "Monorepo". */
  lang: string;
  path: string;
  /** The branch new work starts from. */
  branch: string;
  /** The command that starts the dev server. It may be empty. */
  dev: string;
  lockBypass: boolean;
  /** The package folders of a monorepo. Undefined for any other project. */
  packages: string[] | undefined;
  /** When the project was added, in ms. */
  createdAt: number;
  /** How many cards wait for a person. */
  needs: number;
  /** How many cards have a running agent. */
  awake: number;
}

export function toDaemonProject(project: Project): DaemonProject {
  return {
    id: project.id,
    name: project.name,
    lang: project.language,
    path: project.path,
    branch: project.defaultBranch,
    dev: project.devCommand,
    lockBypass: project.bypassLocked,
    packages: project.isMonorepo ? [...project.packages] : undefined,
    createdAt: toMillis(project.createdAt),
    needs: project.badges.needs,
    awake: project.badges.awake,
  };
}
