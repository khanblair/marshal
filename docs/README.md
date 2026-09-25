# Marshal docs

Marshal is a lightweight desktop app and background service for running AI coding agents on kanban boards. These docs describe what we are building, how, and in what order.

## The docs

| Doc | What it answers |
|---|---|
| `marshal-product-scope.md` | Every feature, how it works, and why |
| `project-overview.md` | What are we building and why? Read this first. |
| `architecture.md` | How is the system built? |
| `project-structure.md` | Which folders and files exist, and what each is for? |
| `build-plan.md` | What do we build, in what order? |
| `code-standards.md` | How do we write code here, including code smells? |
| `development.md` | How do we run, test, and build Marshal? |
| `library-docs.md` | Which libraries do we use, and how? |
| `ui-tokens.md` | What are the design values? |
| `ui-rules.md` | How should the UI look, read, and behave? |
| `ui-registry.md` | Which UI components exist? |
| `design-port.md` | How did the Claude Design become the app, and what differs from it? |
| `backend-checklist.md` | How do we build the daemon and replace the prototype's dummy data, and how is each step tested? |
| `backend-inventory.md` | What does the prototype do, and where does each piece live in the backend? |
| `progress-tracker.md` | Where are we right now? What changed? |

## Reading order

1. `project-overview.md`
2. `progress-tracker.md`, to find the current phase and task
3. `build-plan.md`, for the task's details
4. `architecture.md` and `project-structure.md`, for the parts the task touches
5. `code-standards.md` and `development.md`
   - For backend work: `backend-checklist.md`, then `backend-inventory.md`
6. For UI work: `ui-tokens.md`, `ui-rules.md`, then `ui-registry.md`
7. `library-docs.md`, when using or adding a library
8. `marshal-product-scope.md`, for the full reasoning behind any feature

## Keeping docs true

Any change to features, structure, files, libraries, or decisions updates the matching doc in the same pull request, and is logged in `progress-tracker.md`.
