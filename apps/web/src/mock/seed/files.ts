import type { Card } from "../types";

type Profile = "api" | "web" | "mobile";

const FILES: Record<Profile, readonly string[]> = {
  api: ["internal/proxy/handler.go", "internal/auth/middleware.go", "cmd/gateway/main.go"],
  web: ["src/pages/Reports.tsx", "src/components/Table.tsx", "src/lib/api.ts"],
  mobile: ["src/index.ts", "src/client.ts", "README.md"],
};
const TEST: Record<Profile, string> = {
  api: "go test ./...",
  web: "pnpm test",
  mobile: "pnpm --filter {pkg} test",
};
const LINT: Record<Profile, string> = {
  api: "golangci-lint run",
  web: "pnpm lint",
  mobile: "pnpm lint",
};

/*
 * The prototype only knows the three seeded projects and throws a TypeError for a
 * project added at runtime (sample project, quick add, new card). Unknown projects
 * use the web-dashboard files and commands instead.
 */
const profileOf = (pid: string): Profile => (pid === "api" || pid === "mobile" ? pid : "web");

type CardWhere = Pick<Card, "p" | "pkg">;

export function filesFor(c: CardWhere): readonly string[] {
  const files = FILES[profileOf(c.p)];
  return c.pkg ? files.map((x) => `${c.pkg}/${x}`) : files;
}

export const firstFile = (c: CardWhere): string => filesFor(c)[0] ?? "";

export const testFor = (c: CardWhere): string => TEST[profileOf(c.p)].replace("{pkg}", c.pkg || "");

export const lintFor = (c: CardWhere): string => LINT[profileOf(c.p)];
