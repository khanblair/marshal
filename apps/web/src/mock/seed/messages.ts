import { type IdCounters, takeMid } from "../ids";
import type {
  AgentMsg,
  ApprovalMsg,
  CardRefMsg,
  DiffMsg,
  LinksMsg,
  PlanMsg,
  SystemMsg,
  ToolMsg,
  ToolState,
  UserMsg,
} from "../types";

interface ToolExtra {
  st?: ToolState;
  detail?: string;
}

/** Builds chat messages. Each call takes the next message id, like `m()` in the prototype. */
export interface MsgFactory {
  user(text: string): UserMsg;
  agent(text: string): AgentMsg;
  streaming(): AgentMsg;
  tool(icon: string, action: string, result: string, extra?: ToolExtra): ToolMsg;
  system(text: string): SystemMsg;
  diff(o: Omit<DiffMsg, "id" | "k">): DiffMsg;
  plan(o: Omit<PlanMsg, "id" | "k">): PlanMsg;
  approval(o: Omit<ApprovalMsg, "id" | "k">): ApprovalMsg;
  cardRef(cardId: number): CardRefMsg;
  links(text: string, cards: number[]): LinksMsg;
}

export function createMsgFactory(ids: IdCounters): MsgFactory {
  const id = (): string => `m${takeMid(ids)}`;
  return {
    user: (text) => ({ id: id(), k: "user", text }),
    agent: (text) => ({ id: id(), k: "agent", text }),
    streaming: () => ({ id: id(), k: "agent", text: "", streaming: true }),
    tool: (icon, action, result, extra) => ({
      id: id(),
      k: "tool",
      icon,
      action,
      result,
      st: extra?.st || "ok",
      detail: extra?.detail || "",
      open: false,
    }),
    system: (text) => ({ id: id(), k: "system", text }),
    diff: (o) => ({ id: id(), k: "diff", ...o }),
    plan: (o) => ({ id: id(), k: "plan", ...o }),
    approval: (o) => ({ id: id(), k: "approval", ...o }),
    cardRef: (cardId) => ({ id: id(), k: "card", cardId }),
    links: (text, cards) => ({ id: id(), k: "links", text, cards }),
  };
}
