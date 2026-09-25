import type { Marshal } from "~/mock";

interface RemoveEffect {
  icon: string;
  text: string;
}

/** What the Remove project dialog says about the project it is about to stop managing. */
export interface RemoveModel {
  name: string;
  /** Cards that have a branch and are not done. */
  unmerged: number;
  unmergedLabel: string;
  memoryPath: string;
  effects: RemoveEffect[];
}

/** The dialog's facts, or null when the project no longer exists. */
export function removeModel(M: Marshal, id: string): RemoveModel | null {
  const project = M.proj(id);
  if (!project) return null;
  const cards = M.cardsOf(id);
  const unmerged = cards.filter((c) => c.branch && c.state !== "done").length;
  const awake = cards.filter((c) => M.isAwake(c)).length;
  const chats = (M.S.chats[id] || []).length;
  return {
    name: project.name,
    unmerged,
    unmergedLabel: `${unmerged}${unmerged === 1 ? " card has unmerged work" : " cards have unmerged work"}`,
    memoryPath: `vault/projects/${project.name}/`,
    effects: [
      { icon: "square", text: `Running sessions stop. ${awake} agents are awake now.` },
      { icon: "folder-minus", text: `Worktrees are cleaned up. ${unmerged} worktrees.` },
      {
        icon: "trash-2",
        text: `Cards and chats are removed from Marshal. ${cards.length} cards and ${chats} chats.`,
      },
    ],
  };
}
