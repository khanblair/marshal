/**
 * Fails when a color pair breaks the contrast rules and is not in the accepted
 * exceptions list, or when an accepted exception no longer fails (stale).
 * `--print-exceptions` writes the current failing pairs as a TypeScript list.
 */
import { measureAll, pairKey } from "../src/contrast.ts";
import { contrastExceptions } from "../src/contrast-exceptions.ts";

const failing = measureAll().filter((p) => p.ratio + 1e-9 < p.min);

if (process.argv.includes("--print-exceptions")) {
  for (const p of failing) console.log(`  "${pairKey(p)}",`);
  process.exit(0);
}

const accepted = new Set<string>(contrastExceptions);
const fresh = failing.filter((p) => !accepted.has(pairKey(p)));
const failingKeys = new Set(failing.map(pairKey));
const stale = [...accepted].filter((k) => !failingKeys.has(k));

for (const p of fresh) {
  console.error(`FAIL ${pairKey(p)} ${p.ratio.toFixed(2)} is below ${p.min}`);
}
for (const k of stale) console.error(`STALE exception ${k} passes now. Remove it from the list.`);

console.log(
  `contrast: ${failing.length} failing pairs, ${accepted.size} accepted exceptions, ${fresh.length} new failures, ${stale.length} stale`,
);
process.exit(fresh.length + stale.length > 0 ? 1 : 0);
