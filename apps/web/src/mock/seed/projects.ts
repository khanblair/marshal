import { DAY_MS, MINUTE_MS } from "../constants";
import type { Profile } from "../settings-types";
import type { Project } from "../types";

/** The paired phone was last seen this many minutes before load. */
const PHONE_SEEN_MIN = 40;

export const seedProjects = (): Project[] => [
  {
    id: "api",
    name: "api-gateway",
    lang: "Go",
    path: "~/code/api-gateway",
    ci: "passed",
    ciAgo: 38,
    monthBase: 61.2,
    runs: [
      { wf: "test", st: "passed", ago: 38 },
      { wf: "lint", st: "passed", ago: 38 },
      { wf: "release", st: "cancelled", ago: 310 },
    ],
  },
  {
    id: "web",
    name: "web-dashboard",
    lang: "TypeScript",
    path: "~/code/web-dashboard",
    ci: "running",
    ciAgo: 2,
    monthBase: 48.9,
    runs: [
      { wf: "ci", st: "running", ago: 2 },
      { wf: "e2e", st: "passed", ago: 95 },
      { wf: "deploy-preview", st: "passed", ago: 95 },
    ],
  },
  {
    id: "mobile",
    name: "mobile-app",
    lang: "Monorepo",
    path: "~/code/mobile-app",
    ci: "failed",
    ciAgo: 22,
    monthBase: 88.4,
    packages: ["apps/ios", "apps/android", "packages/ui", "packages/auth", "packages/api-client"],
    runs: [
      { wf: "android", st: "failed", ago: 22, pkg: "apps/android" },
      { wf: "ios", st: "passed", ago: 22, pkg: "apps/ios" },
      { wf: "packages", st: "passed", ago: 22, pkg: "packages/*" },
    ],
  },
];

export const seedProfile = (loadedAt: number): Profile => ({
  name: "Ada Okafor",
  email: "",
  tz: "Europe/London",
  avatar: null,
  tailnet: "ada@kolaborate.co",
  node: "marshal-laptop.tail3f2a.ts.net",
  devices: [
    { id: "d1", name: "Pixel 8", kind: "smartphone", last: loadedAt - PHONE_SEEN_MIN * MINUTE_MS },
    { id: "d2", name: "iPad Air", kind: "tablet", last: loadedAt - 2 * DAY_MS },
  ],
});
