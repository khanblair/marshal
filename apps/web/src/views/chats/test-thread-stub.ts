import { createRenderEffect } from "solid-js";
import type { MsgView } from "~/mock";

/** Which kind of message a view model is, for the stub's `data-kind`. */
function kindOf(item: MsgView): string {
  const flags: [boolean, string][] = [
    [item.isUser, "user"],
    [item.isAgent, "agent"],
    [item.isApproval, "approval"],
    [item.isCard, "card"],
    [item.isLinks, "links"],
    [item.isTool, "tool"],
    [item.isPlan, "plan"],
    [item.isDiff, "diff"],
    [item.isSystem, "system"],
  ];
  return flags.find(([on]) => on)?.[1] ?? "other";
}

/**
 * Stands in for the real `ChatThread` in the view's tests: one `div` per message with its kind
 * and text, so a test can check what the view passes as `items`.
 */
export function ChatThread(props: { items: readonly MsgView[] }): HTMLElement {
  const root = document.createElement("div");
  root.dataset.testid = "thread";
  createRenderEffect(() => {
    root.replaceChildren(
      ...props.items.map((item) => {
        const row = document.createElement("div");
        row.dataset.kind = kindOf(item);
        row.textContent = item.text;
        return row;
      }),
    );
  });
  return root;
}
