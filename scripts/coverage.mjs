/**
 * Checks the daemon's test coverage against the floors in docs/code-standards.md section 6.
 *
 *   node scripts/coverage.mjs
 *
 * `pnpm --filter daemon test` writes `daemon/coverage.out` (the Go coverage profile), so run the
 * tests first. The web app and the UI package have their own floors in their vitest configs, which
 * fail the test run itself.
 *
 * Floors: 70 percent for a daemon package, 85 for the critical ones (harness, security, integrator,
 * session). A package that is below its floor today is listed in `TEMPORARY_FLOORS` with the number
 * it must not fall below and the reason; raise those numbers as the tests catch up, never lower them.
 * `EXEMPT` holds packages with no logic to cover.
 */
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const profilePath = join(root, "daemon", "coverage.out");
const MODULE = "github.com/khanblair/marshal/daemon/";

const DEFAULT_FLOOR = 70;
const CRITICAL_FLOOR = 85;
const CRITICAL = [
  "internal/harness",
  "internal/security",
  "internal/integrator",
  "internal/session",
];

/** Packages with nothing to cover: the programs' entry points and test helpers. */
const EXEMPT = new Map([
  ["cmd/marshal", "the command's entry point"],
  ["cmd/marshald", "the daemon's entry point (composition root)"],
  ["internal/testutil", "test helpers"],
  ["internal/store/db", "code that sqlc generates, exercised through internal/store"],
]);

/** Packages below their floor today: the number each must not fall below, and why. */
const TEMPORARY_FLOORS = new Map([]);

if (!existsSync(profilePath)) {
  console.error("daemon/coverage.out is missing. Run `pnpm --filter daemon test` first.");
  process.exit(1);
}

/** Statements and covered statements for each package, from the profile's lines. */
function readProfile() {
  const packages = new Map();
  for (const line of readFileSync(profilePath, "utf8").split("\n")) {
    if (!line || line.startsWith("mode:")) continue;
    const match = /^(.+):\d+\.\d+,\d+\.\d+ (\d+) (\d+)$/.exec(line);
    if (!match) continue;
    const file = match[1].startsWith(MODULE) ? match[1].slice(MODULE.length) : match[1];
    const pkg = file.slice(0, file.lastIndexOf("/"));
    const total = packages.get(pkg) ?? { statements: 0, covered: 0 };
    total.statements += Number(match[2]);
    if (Number(match[3]) > 0) total.covered += Number(match[2]);
    packages.set(pkg, total);
  }
  return packages;
}

const floorOf = (pkg) =>
  TEMPORARY_FLOORS.get(pkg)?.floor ?? (CRITICAL.includes(pkg) ? CRITICAL_FLOOR : DEFAULT_FLOOR);

let failed = false;
const rows = [...readProfile()].sort(([a], [b]) => a.localeCompare(b));
console.log("Daemon coverage against the floors\n");
for (const [pkg, { statements, covered }] of rows) {
  if (EXEMPT.has(pkg)) continue;
  const percent = statements === 0 ? 100 : (covered / statements) * 100;
  const floor = floorOf(pkg);
  const below = percent < floor;
  failed ||= below;
  const note = TEMPORARY_FLOORS.get(pkg)?.reason ? `  (${TEMPORARY_FLOORS.get(pkg).reason})` : "";
  console.log(
    `${below ? "FAIL" : "ok  "}  ${pkg.padEnd(34)} ${percent.toFixed(1).padStart(5)}%  floor ${floor}%${note}`,
  );
}
if (failed) console.error("\nA package is below its coverage floor. Add tests for the new code.");
process.exit(failed ? 1 : 0);
