import type { SessionState } from "@marshal/protocol";
import type { Msg } from "~/mock";

/** Board state alone does not mean an agent is busy: an awake session can be idle. */
export function agentActivity(
  session: SessionState | null | undefined,
  pending: number,
  messages: readonly Msg[],
): string | undefined {
  if (pending > 0 && session !== "working") return "Sending message";
  if (session === "starting") return "Starting agent";
  if (session === "waking") return "Resuming agent";
  if (session !== "working") return undefined;
  const last = messages.at(-1);
  if (last?.k === "agent" && last.streaming) return "Responding";
  if (last?.k === "tool" && last.st === "running") return "Using tools";
  return "Thinking";
}
