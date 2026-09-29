# Phase 12 report: scale

One evolving file. Update it after every slice, and at the end of every run on this phase, even a stopped one. Never replace it with a fresh file — append and revise in place.

## 1. Summary

Not started.

## 2. Slice results

Not started. One row per slice (named in `phase-prompts/phase-12-scale.md` section 5): status, what it built, tests added, evidence. The first slice is a verify-only pass over List, Timeline, and Calendar (already built in Phases 2 and 8) — record the evidence there, not a rebuild.

## 3. Cutover evidence

Not started. Gate item 8 (`pnpm check` and budgets) is not run by the builder (the full gate runs on GitHub CI): write "not run by the builder; see CI" there, and fill the rest from scoped runs (gate item 8). One row per section against the eight gate items in `docs/backend-checklist.md` 2.4.

| Section | 1. Inventory rows built | 2. Daemon tests + goldens | 3. Mapper tests from goldens | 4. Loading/empty/error/offline | 5. End-to-end specs | 6. Mock removed, knip clean | 7. Registers and docs | 8. `pnpm check` + budgets | Switched |
|---|---|---|---|---|---|---|---|---|---|
| S2c Team and people | | | | | | | | | |
| S5c Package swimlane and filter | | | | | | | | | |

## 4. What I changed

Not started.

## 5. Verification results

Not started. Include the end-of-phase code review update (the graph refresh and the `detect-changes` summary, hard rule 1c in the prompt). Confirm the fixture monorepo works end to end, and the release build is tested on all three platforms (milestone: v1 complete).

## 6. Rulings

Not started. Record here how team mode's second-user authentication was decided (this was an open architectural question at scoping time; see the prompt's section 5).

## 7. Found, not fixed, and later phases

Not started.

## 8. Needs the controller

Not started. Every check that needs a browser goes here as a hands-on checklist for the owner (you never open a browser: owner rule).

## 9. Design-doc rows I added or propose

Not started. The export/import screens (build-plan task 12.10) are noted here for the owner's sign-off.

## 10. Confirmation of the hard rules

Not started.
