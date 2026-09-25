# Design port guide

This document says how the Claude Design prototype in `design/` becomes the real SolidJS frontend. It is the contract for every person or agent working on the port.

**The design is the source of truth.** `design/Marshal.dc.html` is the app shell. It imports the views (`HomeView`, `BoardView`, `CardDetail` and so on) and the fake daemon (`store.js`). Where the design and another doc disagree, the design wins and the other doc is updated to match.

---

## 1. What we build

A frontend only, with dummy data. No backend, no daemon, no Tauri shell.

| Design file | Becomes |
|---|---|
| `store.js` (fake daemon, seed data, actions, timers) | `apps/web/src/mock/`, exposed as `M` and as `window.M` |
| `Marshal.dc.html` (shell) | `apps/web/src/app/` |
| `HomeView`, `HomeAllView` | `apps/web/src/views/home/` |
| `BoardView`, `CardItem` | `apps/web/src/views/board/` |
| `CardDetail`, `ChatThread` | `apps/web/src/views/card/` |
| `ChatsView` | `apps/web/src/views/chats/` |
| `AgentsView`, `ListView` | `apps/web/src/views/agents/`, `views/list/` |
| `TimelineView`, `CalendarView` | `apps/web/src/views/timeline/`, `views/calendar/` |
| `SettingsView` | `apps/web/src/views/settings/` |
| `Onboarding`, `Tour` | `apps/web/src/onboarding/`, `apps/web/src/features/tutorial/` |
| Repeated styled elements | `packages/ui/src/` |
| Design colors, type, radius, motion | `packages/tokens/` |

Out of scope: `Marshal Showcase.dc.html` (two marketing posters), and the prototype's own floating toolbar in `Marshal.dc.html` (Fit window, Phone, Tablet, Desktop, Reset to first launch). The toolbar was first included, then removed on request on 2026-09-25: the app always fills the window and its size follows the window, driven by `M.S.vw` and `M.S.vh` (`apps/web/src/app/use-window-viewport.ts`). The parity harness hides the toolbar in the prototype so both sides compare the same screen.

## 2. Translating a template

The prototype uses a small template language on top of React. Translate it like this.

| Design | Solid |
|---|---|
| `{{ expr }}` | A JSX expression |
| `<sc-if value="{{ x }}">` | `<Show when={x}>` |
| `<sc-for list="{{ xs }}" as="x">` | `<For each={xs}>{(x) => ...}</For>` |
| `<dc-import name="Foo" prop="{{ v }}">` | `<Foo prop={v} />` |
| `style="..."` | Tailwind classes that map to tokens (section 3) |
| `style-hover="..."` | `hover:` variants |
| `<m-icon name="x" size="16">` | `<Icon name="x" size={16} />` from `@marshal/ui` |
| `<m-dot state="working">` | `<StatusDot state="working" />` from `@marshal/ui` |
| `ref="{{ fn }}"` | `ref={fn}` |
| `onClick`, `onInput`, `onKeyDown` | Same names. `onChange` on a select or checkbox stays `onChange`. |
| `defaultValue` | `value` set once (uncontrolled) |
| React re-render on `M.on(...)` | Nothing. `M.S` is a reactive store. |

**Keep these attributes exactly.** Other code finds elements by them: `data-tour`, `data-card`, `data-col`, `data-no-nav`, `data-app-root`, `data-search`, `data-pi`, `data-compact`, `data-size`, `data-touch`, `role`, `aria-*`, and `title`.

**Keep every string exactly.** Labels, placeholders, messages, and titles come from the design. Do not reword them.

**Keep every behavior.** Keyboard shortcuts, focus handling, drag and drop, menus that close on outside click, and the timed simulation in the store all stay.

## 3. Styling

- Use **Tailwind classes** that map to tokens. Colors come from the theme: `bg-surface`, `text-secondary`, `border-border-strong`, `text-status-working-text`, `bg-status-needs-you-subtle`.
- **Spacing is a scale of 4 px.** `p-3` is 12 px, `gap-1.5` is 6 px, `h-8` is 32 px, `p-3.5` is 14 px. Any multiple of 0.5 works, so a 2 px step is always available.
- **Radius tokens:** `rounded-xs` 3 px, `rounded-sm` 5 px, `rounded-md` 7 px, `rounded-lg` 10 px, `rounded-xl` 14 px, `rounded-full`. The design also uses 2 px (`rounded-2xs`) and 6 px (`rounded-sm-plus`).
- **Font sizes** are tokens that set only the size: `text-micro` 10, `text-badge` 11, `text-caption` 12, `text-small` 13, `text-body` 14, `text-lead` 15, `text-subtitle` 16, `text-title` 18, `text-view-title` 22, `text-display` 24, `text-tile` 28.
- **Line height is separate.** The design sets `line-height: 20px` on the app root, and a `font-size` alone inherits it. Only add `leading-*` where the design states a line height: 16 px `leading-4`, 18 px `leading-4.5`, 20 px `leading-5`, 22 px `leading-5.5`, 24 px `leading-6`, 28 px `leading-7`.
- **Weights:** `font-normal` 400, `font-medium` 500, `font-semibold` 600, `font-bold` 700.
- **Shadows:** `shadow-e1`, `shadow-e2`, `shadow-drag`.
- **Layers:** `z-raised`, `z-sticky`, `z-bar`, `z-banner`, `z-menu`, `z-detail`, `z-side`, `z-scrim`, `z-dialog`, `z-sheet`, `z-palette`, `z-toast`, `z-tour`, `z-onboarding`. Use `z-[155]` only for a one-off number the design uses.
- **Tailwind preflight is off.** The design was drawn against a minimal reset and the browser defaults. Do not rely on Tailwind resets. Copy every explicit declaration the design makes, such as `border:none`, `margin:0`, and `padding:0`.
- **No raw hex colors and no font names in components.** If a color is missing, add it to `packages/tokens/src/themes.ts` or `staticColors` in `tokens.ts` (append only), then run `pnpm gen`.
- **Arbitrary values** such as `min-w-[560px]` or `grid-cols-[repeat(auto-fit,minmax(150px,1fr))]` are fine when the design uses a value no token or scale covers.
- **Inline `style`** is only for values computed at run time: a drag position, a chart coordinate, a status color chosen from data.
- Responsive rules come from `M.S.vw`, exactly as the design does it (phone under 640, tablet 640 to 1199, desktop 1200 and up). Do not use CSS breakpoints for layout that the design drives from `vw`, because the prototype can render a phone frame inside a large window.

## 4. Reactivity

`M.S` is a Solid mutable store (`createMutable`). Read it inside JSX, `createMemo`, or `createEffect` and the view updates by itself. Call the actions on `M` to change it. Never copy a store value into a signal.

`M.deco(card)` returns the view model for a card. Wrap it in a memo per card: `const d = createMemo(() => M.deco(card))`.

## 5. Code standards

Read `code-standards.md`. The limits that trip CI:

| Check | Blocking |
|---|---|
| Function length | Over 100 lines (aim for 50) |
| File length | Over 800 lines (aim for 400) |
| Parameters | Over 6 (aim for 4) |
| Nesting depth | Over 5 (aim for 3) |
| Cognitive complexity | Over 25 (aim for 15) |
| Duplicated code | 30 lines or more (10 warns) |
| Unused code, exports, and dependencies | Any |

The design's own code breaks these limits in places (`renderVals` in the shell is over 200 lines). Split it into small components, hooks, and helpers. No magic numbers: name them, with units in the name. No `any`. Comments say why, not what.

## 6. Tests

Every component or module with logic has a Vitest test next to it (`Foo.test.tsx`). Views render with the mock store and assert the key text, roles, and one or two interactions. Coverage floor: 60 percent for `apps/web` and `packages/ui`.

## 7. Parity check

`pnpm parity` renders the original prototype and the port with the same frozen clock, the same size and theme, and the same steps, then compares screenshots pixel by pixel and compares the visible text line by line. Scenarios live in `tools/parity/scenarios/`. A view is done when its scenarios pass at phone (390 by 844), tablet (820 by 1180), and desktop (1440 by 900), in both themes.

### Known deviations from the rendered prototype

Approved by the project owner. Each is hidden or handled in the parity harness so the rest of the screen still compares exactly.

| Deviation | Why | Handling in `pnpm parity` |
|---|---|---|
| Home charts draw axis labels and the limit label | The prototype's template engine wraps every interpolation in an HTML span, and an HTML span inside an SVG `text` element is never drawn, so the rendered prototype has no labels. The design's markup asks for them, and a chart without a scale is not readable. Approved 2026-09-25. | Chart text (`svg[role="img"] text`) is hidden in both apps |
| No prototype toolbar | Removed on request, 2026-09-25. | Hidden in the prototype |

## 8. Rules for agents working on the port

1. Read this guide, then the design files you own. Read `store.js` for any `M.*` function you call.
2. Only create or edit files in the folders you own. If you need a change in a shared file (`tokens.ts`, `themes.ts`), make an append-only edit.
3. Do not create commits. There is no repository yet.
4. Reuse `@marshal/ui` components. If two places in the design share a pattern, it becomes a component, not a copy.
5. Update `docs/ui-registry.md` when you add a component, and list new files in your report so `docs/project-structure.md` can be updated.
6. If the design conflicts with any doc, follow the design and list the conflict in your report.
7. Finish with a short report: what you built, tests run and their result, parity result, deviations from the design, and conflicts with docs.
