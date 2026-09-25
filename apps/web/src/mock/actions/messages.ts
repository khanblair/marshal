import type { Ctx } from "../context";
import { later, pushMsg, setState, streamCard } from "../engine";
import { firstFile } from "../seed/files";
import { card } from "../selectors";
import type { Card } from "../types";
import { startSession } from "./sessions";

/** Delay before an awake agent answers a message. */
const REPLY_MS = 500;
/** How long waking a sleeping session takes before it answers. */
const WAKE_FOR_REPLY_MS = 1800;

function reply(ctx: Ctx, c: Card): void {
  const wasNeeds = c.state === "needs";
  c.doing = "Reading your message";
  if (c.state !== "done") setState(ctx, c, "working");
  const text = wasNeeds
    ? "Thanks, that answers it. I'll continue with that approach and run the tests again."
    : "Got it. I'll fold that into the current change and tell you when the tests pass.";
  streamCard(ctx, c.id, text, () => {
    c.doing = `Editing ${firstFile(c)}`;
  });
}

/** Sends a message to a card's agent. A backlog card starts; a sleeping one wakes first. */
export function send(ctx: Ctx, id: number, text: string): void {
  const c = card(ctx, id);
  if (!c || !text.trim()) return;
  pushMsg(ctx, id, ctx.msg.user(text));
  if (c.state === "backlog") {
    startSession(ctx, c);
    return;
  }
  if (!c.asleep) {
    later(REPLY_MS, () => reply(ctx, c));
    return;
  }
  c.waking = true;
  pushMsg(ctx, id, ctx.msg.system("Waking the session. This takes a few seconds."));
  later(WAKE_FOR_REPLY_MS, () => {
    c.waking = false;
    c.asleep = false;
    reply(ctx, c);
  });
}
