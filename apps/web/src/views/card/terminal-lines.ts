import { type Card, M, type Msg } from "~/mock";

/** One line of the fake terminal: its text and the classes that color it. */
export interface TerminalLine {
  text: string;
  class: string;
}

const SESSION_ID_FACTOR = 7919;
const HEX_RADIX = 16;
const SECONDARY = "text-secondary";
const NEEDS_YOU = "text-status-needs-you-text font-semibold";

function headerLines(card: Card): TerminalLine[] {
  // A card can name an agent the catalog does not know (mock cards), or one that is not installed and has no version.
  const version = M.AGENTS[card.agent]?.version;
  const project = M.proj(card.p);
  const session = `${(card.n * SESSION_ID_FACTOR).toString(HEX_RADIX)}a2f`;
  const where = card.branch
    ? `~/.marshal/worktrees/${project?.name}/${card.branch.replace("marshal/", "")}`
    : project?.path;
  return [
    {
      text: `${version ? `${card.agent} ${version}` : card.agent}   session ${session}`,
      class: SECONDARY,
    },
    { text: `Resumed in ${where}`, class: SECONDARY },
    { text: " ", class: "" },
  ];
}

function messageLines(msg: Msg): TerminalLine[] {
  switch (msg.k) {
    case "user":
      return [{ text: `> ${msg.text}`, class: "text-primary font-semibold" }];
    case "agent":
      return [{ text: msg.text, class: "text-primary" }];
    case "tool":
      return [
        { text: `  ${msg.action}`, class: "text-primary" },
        {
          text: `    ${msg.result}`,
          class: msg.st === "fail" ? "text-status-danger-text" : SECONDARY,
        },
      ];
    case "system":
      return [{ text: `# ${msg.text}`, class: "text-muted" }];
    case "approval":
      return [
        {
          text: `? Allow: ${msg.cmd}${msg.st === "waiting" ? "  [y/n]" : `  ${msg.st}`}`,
          class: NEEDS_YOU,
        },
      ];
    case "plan":
      return [{ text: `? Plan with ${msg.steps.length} steps  ${msg.st}`, class: NEEDS_YOU }];
    default:
      return [];
  }
}

/**
 * The terminal's text: a session header, the chat as plain lines, then the keys pressed
 * on the phone key bar. The terminal is fake text, as in the design.
 */
export function terminalLines(
  card: Card,
  chat: readonly Msg[],
  keyLog: readonly string[],
): TerminalLine[] {
  return [
    ...headerLines(card),
    ...chat.flatMap(messageLines),
    ...keyLog.map((key) => ({ text: `[${key}]`, class: "text-muted" })),
  ];
}
