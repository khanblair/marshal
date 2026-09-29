# Phase prompts

Working files for handing each remaining phase to any builder agent or tool, one file per phase, in `build-plan.md` order. **Not part of the product**: never committed, and not listed in `docs/project-structure.md`.

Each prompt is a self-contained brief: it names the phase's items from `docs/backend-checklist.md`, the owner's decisions and open questions, what to build, in what order, and what to verify. Every prompt tells the builder to first check whether an item was already built early by an earlier phase (`docs/backend-checklist.md` section 2.7) — skip and verify those with evidence, and only build what is genuinely missing.

Phase 0 (Foundation) and Phase 1 (First card) are already done and pushed; they have no prompt here. Phase 2 (Core UI) has no prompt or report here: all sixteen of its sections are switched to the daemon (see `docs/backend-checklist.md` 2.2 and `docs/architecture.md` "State today"), and its working files were deleted. What is verified and what is not is in `docs/progress-tracker.md`; the full gate, the browser suite, and the owner's own check still have to run.

**This is one continuous run, not a gated pipeline.** The owner's decision (2026-09-26): the builder works through Phases 3 to 13 in order, one file after the next, without stopping for the controller between phases — each prompt ends with a "Then continue" line pointing at the next file. The controller verifies afterward, not before every phase starts. Within a phase, subagents (if the harness has them) draft — files, research, first-pass tests — and the main agent alone ever builds, tests, or reconciles a shared file; see each phase's own "Using your own subagents" section.

**Code review graph.** The repo has a local code knowledge graph (`.code-review-graph/`, git-ignored) with its MCP server configured in `.mcp.json`. Every prompt tells the builder to use it when searching, before and after changing shared code, and to run a code review update (`code-review-graph update`, then `detect-changes`) when the phase is done, recording the result in its report. Without MCP support the same graph is available from the shell as `code-review-graph <command>`.

**Only fast checks are run locally.** Owner rule (2026-09-26): nobody runs the full gate (`node scripts/check.mjs`, the whole-repo test suites, `pnpm build`, `pnpm budgets`, the browser suite) on the owner's machine unless the owner asks, and nobody starts a process to check work by hand (no daemon, no smoke script, no dev server, no browser). GitHub CI runs everything else on every pull request. Builders verify with typecheck, `biome check` and `golangci-lint` on what they touched, and the fast unit tests of the files they touched, and they write "not run (the owner has not asked)" in their report for the rest. Every prompt says so (rule 1d).

**No browser, ever.** Owner rule (2026-09-26): no builder, subagent, or controller opens a browser or a preview, or runs `pnpm test:e2e`, unless the owner asks. Every prompt says so, and a check that needs a browser goes in the report under "Needs the controller" for the owner's own hands-on check.

| File | Phase | Goal | Status |
|---|---|---|---|
| [phase-03-control-and-safety.md](phase-03-control-and-safety.md) | 3. Control and safety | Users can trust agents with their code | Report written |
| [phase-04-models.md](phase-04-models.md) | 4. Models | The built-in agent and full provider support | Report written |
| [phase-05-quality-loop.md](phase-05-quality-loop.md) | 5. Quality loop | Idea to merged code, with approvals only | Report written |
| [phase-06-ci-and-preview.md](phase-06-ci-and-preview.md) | 6. CI and preview | The screens show real CI, and cards can be previewed | Report written |
| [phase-07-orchestration-and-memory.md](phase-07-orchestration-and-memory.md) | 7. Orchestration and memory | Agents see the board and each other, and remember | Report written; needs controller verification |
| [phase-08-automation.md](phase-08-automation.md) | 8. Automation | Scheduled work, and Trello, Calendar, and Gmail connected for real | In progress: slice 1 done, slice 2 started |
| [phase-09-remote.md](phase-09-remote.md) | 9. Remote | Everything works from a phone | Report written; slices 1-4 done, slice 5 partial |
| [phase-10-advanced-cards.md](phase-10-advanced-cards.md) | 10. Advanced cards | The card features the prototype already shows are real | Not started |
| [phase-11-ecosystem.md](phase-11-ecosystem.md) | 11. Ecosystem | Skills, MCP servers, and plugins | Not started |
| [phase-12-scale.md](phase-12-scale.md) | 12. Scale | The remaining views and modes are backed by the daemon | Not started |
| [phase-13-retire-the-mock-and-accept.md](phase-13-retire-the-mock-and-accept.md) | 13. Retire the mock and accept | The app runs only on the daemon | Not started |

Each phase's matching report is `phase-reports/<same file name>.md`.
