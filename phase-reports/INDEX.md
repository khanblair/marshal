# Phase reports

One evolving report per phase, written by whoever builds that phase (whichever agent or tool builds it), matching the briefs in `phase-prompts/`. **Not part of the product**: never committed, and not listed in `docs/project-structure.md`.

A report is a single file that gets appended to and updated across however many runs a phase takes — it is never replaced by a fresh dated file. Its ten sections mirror the ones in each phase prompt's "report" instructions: summary, slice results, cutover (or retirement) evidence, what changed, verification results, rulings, found-not-fixed and later phases, needs the controller, design-doc rows, and confirmation of the hard rules.

Phase 0 and Phase 1 have no report here (done and pushed, recorded in `docs/progress-tracker.md`). Phase 2 has no report here either (its working files were deleted).

A report also lists, under "Needs the controller", every check that needs a browser: builders never open one (owner rule, 2026-09-26).

| File | Phase | Status |
|---|---|---|
| [phase-03-control-and-safety.md](phase-03-control-and-safety.md) | 3. Control and safety | Report written; slices 1-5 done per the report |
| [phase-04-models.md](phase-04-models.md) | 4. Models | Report written; code-complete, all three sections switched |
| [phase-05-quality-loop.md](phase-05-quality-loop.md) | 5. Quality loop | Report written; code-complete |
| [phase-06-ci-and-preview.md](phase-06-ci-and-preview.md) | 6. CI and preview | Report written; code-complete |
| [phase-07-orchestration-and-memory.md](phase-07-orchestration-and-memory.md) | 7. Orchestration and memory | Report written; needs controller verification (3 items built-not-switched) |
| [phase-08-automation.md](phase-08-automation.md) | 8. Automation | In progress: slice 1 done (daemon side), slice 2 started |
| [phase-09-remote.md](phase-09-remote.md) | 9. Remote | Report written; slices 1-4 done, slice 5 partial (router built but not on the bus; remote machines and status screens not built; S29f/S29g switched) |
| [phase-10-advanced-cards.md](phase-10-advanced-cards.md) | 10. Advanced cards | Not started |
| [phase-11-ecosystem.md](phase-11-ecosystem.md) | 11. Ecosystem | Not started |
| [phase-12-scale.md](phase-12-scale.md) | 12. Scale | Not started |
| [phase-13-retire-the-mock-and-accept.md](phase-13-retire-the-mock-and-accept.md) | 13. Retire the mock and accept | Not started |

The controller (this session, or whoever verifies) reads a report, diffs the code against a saved baseline, looks at what GitHub CI reports for the phase (the full gate and the browser suite run there; neither the builder nor the controller runs them on the owner's machine unless the owner asks), lists the hands-on browser pass for the owner, then ticks the phase's items in `docs/backend-checklist.md` and moves its row in `docs/progress-tracker.md` to Done. A phase's builder never ticks the checklist itself.
