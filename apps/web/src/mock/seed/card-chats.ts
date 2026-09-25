import type { CardKey } from "../card-key";
import type { Card, Msg } from "../types";
import { genericChat } from "./generic";
import type { MsgFactory } from "./messages";

/** #210 keeps its generated chat and adds the merge conflict to it. */
const CONFLICT_CARD_ID: CardKey = "mobile#210";

const chat41 = (b: MsgFactory): Msg[] => [
  b.user(
    "Users get logged out when the access token expires during a request. Refresh once and retry the request. Keep the change inside internal/auth.",
  ),
  b.agent("I'll start with the token middleware and the refresh client."),
  b.tool("file-search", "Read internal/auth/middleware.go", "212 lines"),
  b.tool("file-search", "Read internal/auth/refresh.go", "148 lines"),
  b.agent(
    "The middleware refreshes on every 401. Two parallel requests start two refreshes, and the second one invalidates the first token. I'll add a single-flight guard so concurrent requests share one refresh.",
  ),
  b.tool("file-pen", "Edited internal/auth/refresh.go", "+38 −9", {
    st: "ok",
    detail:
      'func (c *Client) Refresh(ctx context.Context) (Token, error) {\n+\tv, err, _ := c.group.Do("refresh", func() (any, error) {\n+\t\treturn c.refreshOnce(ctx)\n+\t})',
  }),
  b.tool("file-pen", "Edited internal/auth/middleware.go", "+19 −3"),
  b.tool("file-plus", "Created internal/auth/refresh_test.go", "+7 −0"),
  b.diff({ files: 3, add: 64, del: 12 }),
  b.tool("terminal", "Ran go test ./internal/auth/...", "Running", { st: "running" }),
];

const chat43 = (b: MsgFactory): Msg[] => [
  b.user(
    "Add per-key rate limiting to the proxy. Limits come from the plan tier. Plan first, I want to see it before any code.",
  ),
  b.agent(
    "I read the proxy middleware, the config loader, and the tier definitions. Here is my plan.",
  ),
  b.tool("file-search", "Read internal/proxy/middleware.go", "301 lines"),
  b.tool("file-search", "Read internal/config/tiers.go", "64 lines"),
  b.plan({
    st: "waiting",
    editing: false,
    steps: [
      "Add a token bucket limiter keyed by API key in internal/ratelimit",
      "Load limits per plan tier from config, with a default of 100 requests per minute",
      "Run the limiter in the proxy middleware, after key lookup and before the upstream call",
      "Return 429 with a Retry-After header when a key is over its limit",
      "Add unit tests and a benchmark for the limiter",
    ],
    files: [
      "internal/ratelimit/bucket.go",
      "internal/ratelimit/bucket_test.go",
      "internal/proxy/middleware.go",
      "internal/config/tiers.go",
    ],
    risks: [
      "Limits are per instance until shared state is added, so 3 pods allow 3 times the limit",
      "A hot key could lock the shared map. I'll shard the map by key hash",
    ],
    checks: ["go test ./...", "golangci-lint run", "Limiter benchmark under 200 ns per call"],
  }),
];

const chat44 = (b: MsgFactory): Msg[] => [
  b.user("Upgrade grpc-go to 1.66 and fix anything that breaks."),
  b.tool("file-search", "Read go.mod", "48 lines"),
  b.agent(
    "grpc-go is on v1.64.1. Upgrading changes go.mod and go.sum, so I need your approval to run the command.",
  ),
  b.approval({
    st: "waiting",
    cmd: "go get google.golang.org/grpc@v1.66.0",
    why: "Changes go.mod and go.sum in this worktree. Needs network access to proxy.golang.org.",
  }),
];

const TSC_SAME_ERROR = "columns.tsx:48:7 - error TS2322 (same error)";

const chat119 = (b: MsgFactory): Msg[] => [
  b.user("Move all dashboard tables to TanStack Table v8."),
  b.tool("file-pen", "Edited src/components/table/columns.tsx", "+120 −88"),
  b.tool("terminal", "Ran pnpm tsc --noEmit", "Failed", {
    st: "fail",
    detail:
      "src/components/table/columns.tsx:48:7 - error TS2322: Type 'AccessorFn<Row, unknown>' is not assignable to type 'AccessorFn<Row, string>'.",
  }),
  b.tool("file-pen", "Edited src/components/table/columns.tsx", "+4 −4"),
  b.tool("terminal", "Ran pnpm tsc --noEmit", "Failed", { st: "fail", detail: TSC_SAME_ERROR }),
  b.tool("file-pen", "Edited src/components/table/columns.tsx", "+4 −4"),
  b.tool("terminal", "Ran pnpm tsc --noEmit", "Failed", { st: "fail", detail: TSC_SAME_ERROR }),
  b.system(
    "The stuck detector paused this card. The same type error appeared 3 times in a row in columns.tsx.",
  ),
  b.agent(
    "I keep hitting the same generic type error on the accessor. Should I type the column helper as createColumnHelper<ReportRow>(), or keep the loose typing and cast at the call site?",
  ),
];

const chat210 = (b: MsgFactory, card: Card): Msg[] => [
  ...genericChat(card, b),
  b.system(
    "The Integrator stopped the merge. The dry run found a conflict it can't resolve safely.",
  ),
  b.agent(
    "Both #210 and #208 changed packages/api-client/src/queue.ts. #208 renamed QueueError to ApiClientError, and #210 added retries that throw QueueError. Resolving by intent means switching #210 to ApiClientError, but that changes the public error type for apps/ios. How should I proceed?",
  ),
];

const chat209 = (b: MsgFactory): Msg[] => [
  b.user(
    "Add biometric login on Android with a PIN fallback. This is a spike, run without stopping.",
  ),
  b.system(
    "Bypass permissions is on for this card. The agent can run any command in its worktree without asking.",
  ),
  b.tool("terminal", "Ran ./gradlew :app:dependencies", "Done"),
  b.tool(
    "file-plus",
    "Created apps/android/app/src/main/java/app/auth/BiometricPromptManager.kt",
    "+142 −0",
  ),
  b.tool("file-pen", "Edited apps/android/app/build.gradle.kts", "+2 −0"),
  b.agent("The prompt and fallback are wired up. I'm running the unit tests now."),
  b.tool("terminal", "Ran ./gradlew :app:testDebugUnitTest", "Running", { st: "running" }),
];

const chat118 = (b: MsgFactory): Msg[] => [
  b.user("Settings page ignores dark mode. Fix it using the existing theme tokens."),
  b.tool("file-search", "Read src/pages/settings.tsx", "288 lines"),
  b.agent("Settings uses hard-coded grays in 14 places. I'm replacing them with theme tokens."),
  b.tool("file-pen", "Edited src/pages/settings.tsx", "+31 −31"),
  b.tool("terminal", "Ran pnpm test settings", "18 passed"),
];

const chat46 = (b: MsgFactory): Msg[] => [
  b.user(
    "internal/config is 1,900 lines. Split it into smaller packages without changing behavior. Plan first.",
  ),
  b.tool("file-search", "Read internal/config/config.go", "1,904 lines"),
  b.agent("I'm tracing which packages call config before I write a plan."),
];

/**
 * Chats for every seeded card. Like the prototype, every card first gets a generated
 * chat, then eight cards get a written one; the discarded ids keep later ids aligned.
 */
export function seedCardChats(cards: Card[], b: MsgFactory): Record<CardKey, Msg[]> {
  const chat: Record<CardKey, Msg[]> = {};
  for (const c of cards) chat[c.id] = genericChat(c, b);
  const card210 = cards.find((c) => c.id === CONFLICT_CARD_ID);
  chat["api#41"] = chat41(b);
  chat["api#43"] = chat43(b);
  chat["api#44"] = chat44(b);
  chat["web#119"] = chat119(b);
  if (card210) chat[CONFLICT_CARD_ID] = chat210(b, card210);
  chat["mobile#209"] = chat209(b);
  chat["web#118"] = chat118(b);
  chat["api#46"] = chat46(b);
  return chat;
}
