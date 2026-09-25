// biome-ignore-all lint/style/noMagicNumbers: seed data table, values are the prototype's fake data
import { MINUTE_MS } from "../constants";
import { type IdCounters, takeMid } from "../ids";
import type { Chat, Msg } from "../types";
import type { MsgFactory } from "./messages";

interface ChatSeed {
  pid: string;
  title: string;
  target: string;
  /** Built right before the chat takes its id, the order the prototype uses. */
  msgs: () => Msg[];
  ago: number;
  archived?: boolean;
}

interface Starters {
  api: Msg[];
  web: Msg[];
  mobile: Msg[];
}

const DAYS = 24 * 60;

/** The first threads of each project, built before any chat takes an id. */
const starterThreads = (b: MsgFactory): Starters => ({
  api: [
    b.user("Plan the rate limiting work. I want per-key limits based on plan tier."),
    b.agent(
      "I looked at the proxy middleware and the tier config. This fits in one card. I created it in plan first mode, so you'll see a plan before any code.",
    ),
    b.cardRef(43),
    b.user("Also make a card to upgrade grpc-go, we're two minor versions behind."),
    b.cardRef(44),
    b.agent("#44 is waiting on your approval to change go.mod."),
    b.approval({
      st: "waiting",
      cmd: "go get google.golang.org/grpc@v1.66.0",
      why: "Requested by #44 Upgrade grpc-go to 1.66",
      cardId: 44,
    }),
  ],
  web: [
    b.user("What is blocked?"),
    b.links(
      "One card is blocked. #119 is stuck on the same type error in columns.tsx and is asking how to type the column helper. #121 waits for #119 to merge.",
      [119, 121],
    ),
  ],
  mobile: [
    b.user("Why is main failing?"),
    b.agent(
      "The android workflow failed on main 22 minutes ago in LoginFlowTest. I started #213 with the Tester role to fix it. It is reading the failed step log now.",
    ),
    b.cardRef(213),
  ],
});

const apiChats = (b: MsgFactory, s: Starters): ChatSeed[] => [
  {
    pid: "api",
    title: "Rate limiting per key",
    target: "Orchestrator",
    msgs: () => s.api.slice(0, 3),
    ago: 34,
  },
  {
    pid: "api",
    title: "Upgrade grpc-go",
    target: "Orchestrator",
    msgs: () => s.api.slice(3),
    ago: 12,
  },
  {
    pid: "api",
    title: "Load test results",
    target: "Tester",
    msgs: () => [
      b.user("How did the last load test on /v2/proxy go?"),
      b.agent(
        "p99 was 182 ms at 500 requests per second. #42 is running the 1,000 requests per second step now.",
      ),
      b.cardRef(42),
    ],
    ago: 90,
  },
  {
    pid: "api",
    title: "JWKS caching question",
    target: "Orchestrator",
    msgs: () => [
      b.user("Do we cache JWKS keys?"),
      b.agent("Yes. #33 added a 10 minute cache and merged 2 days ago."),
      b.cardRef(33),
    ],
    ago: 2 * DAYS,
    archived: true,
  },
];

const otherChats = (b: MsgFactory, s: Starters): ChatSeed[] => [
  { pid: "web", title: "What is blocked", target: "Orchestrator", msgs: () => s.web, ago: 20 },
  {
    pid: "web",
    title: "Settings dark mode",
    target: "#118",
    msgs: () => [
      b.user("Use the same tokens as the reports page."),
      b.agent("Understood. I am reusing the report tokens for all 14 hard-coded grays."),
    ],
    ago: 6,
  },
  {
    pid: "mobile",
    title: "Why main is failing",
    target: "Orchestrator",
    msgs: () => s.mobile,
    ago: 22,
  },
  {
    pid: "mobile",
    title: "Button variants review",
    target: "Reviewer",
    msgs: () => [
      b.user("Review #207 when it is ready."),
      b.agent(
        "I left 2 comments on pull request #1432. Both are about focus styles on the ghost variant.",
      ),
      b.cardRef(207),
    ],
    ago: 44,
  },
];

export function seedProjectChats(
  b: MsgFactory,
  ids: IdCounters,
  loadedAt: number,
): Record<string, Chat[]> {
  const starters = starterThreads(b);
  const chats: Record<string, Chat[]> = {};
  for (const seed of [...apiChats(b, starters), ...otherChats(b, starters)]) {
    const msgs = seed.msgs();
    const chat: Chat = {
      id: `ch${takeMid(ids)}`,
      pid: seed.pid,
      title: seed.title,
      target: seed.target,
      msgs,
      last: loadedAt - seed.ago * MINUTE_MS,
      archived: !!seed.archived,
    };
    chats[seed.pid] = [...(chats[seed.pid] ?? []), chat];
  }
  return chats;
}
