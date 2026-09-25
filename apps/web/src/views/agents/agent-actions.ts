import { type Card, M } from "~/mock";

/** One button at the end of a session row. */
export interface AgentAction {
  label: string;
  icon: string;
  /** Names the card for screen readers, such as `Wake #117`. */
  aria: string;
  run: () => void;
}

export function openChatAction(): AgentAction {
  return {
    label: "Open chat",
    icon: "message-square",
    aria: "Open project chat",
    run: () => M.setView("chat"),
  };
}

/** Stops the agent process after a confirm. The card keeps its session and goes to sleep. */
export function stopSession(card: Card): void {
  M.confirm({
    title: "Stop session",
    message: `This stops the agent process for #${card.id}. The session is kept, and you can resume it later.`,
    action: "Stop session",
    run: () => {
      card.paused = card.state === "working";
      card.asleep = true;
      card.doing = "";
      M.toast("Session stopped");
    },
  });
}

/** Open always; the others only while the card has not finished. Read it inside a memo. */
export function actionsFor(card: Card): AgentAction[] {
  const id = card.id;
  const actions: AgentAction[] = [
    { label: "Open", icon: "panel-right-open", aria: `Open #${id}`, run: () => M.openCard(id) },
  ];
  if (card.state === "done") return actions;
  actions.push(
    card.asleep
      ? { label: "Wake", icon: "sun", aria: `Wake #${id}`, run: () => M.wake(id) }
      : { label: "Sleep", icon: "moon", aria: `Sleep #${id}`, run: () => M.sleep(id) },
    {
      label: card.pinned ? "Unpin" : "Pin",
      icon: card.pinned ? "pin-off" : "pin",
      aria: `${card.pinned ? "Unpin" : "Pin"} #${id}`,
      run: () => M.pin(id),
    },
  );
  if (!card.asleep) {
    actions.push({
      label: "Stop",
      icon: "square",
      aria: `Stop session on #${id}`,
      run: () => stopSession(card),
    });
  }
  return actions;
}
