# UI tokens

This document holds every design value in Marshal: colors, type, spacing, radius, elevation, motion, and layout sizes. Components use these tokens only. No raw hex values, pixel sizes, or font names appear in component code.

The source of truth for the values is `packages/tokens`. This document explains them and must stay in sync with that package.

---

## 1. Design direction

### 1.1 The idea: track side

A marshal at a race track controls a busy course with a small set of colored flags. Green means go. Yellow means caution. Blue means someone needs to pass. Red means stop. The checkered flag means finished. Everyone reads them instantly, because color is only ever used to signal.

Marshal's interface works the same way:

- **The surface is quiet.** Cool neutrals, called chalk in the light theme and asphalt in the dark theme, carry the layout.
- **Color only signals status.** Card states use a small set of flag colors, and nothing else in the interface competes with them.
- **Primary actions use ink**, the strongest neutral, not a brand color. This keeps every colored thing on screen meaningful.
- **Done work goes quiet.** Finished cards take the checkered flag, which is black and white, so the board's color always points at work that is still moving or needs you.

This is the one bold choice in the system. Everything else stays calm and disciplined around it.

### 1.2 Why not a typical palette

We deliberately avoid common defaults: a warm cream background with a clay accent, a near-black background with one acid-green accent, and a brand color splashed on every button. In a status-heavy tool, a decorative brand color would compete with the status colors, and the board would stop being readable at a glance.

---

## 2. Color

### 2.1 Neutrals

| Token | Light (chalk) | Dark (asphalt) | Use |
|---|---|---|---|
| `color-canvas` | `#F5F6F7` | `#15181C` | App background behind everything |
| `color-surface` | `#FFFFFF` | `#1B1F24` | Cards, panels, main content |
| `color-surface-sunken` | `#EEF0F2` | `#111417` | Board columns, wells, code blocks |
| `color-surface-raised` | `#FFFFFF` | `#22272D` | Popovers, menus, dialogs |
| `color-surface-hover` | `#F0F2F4` | `#242A31` | Hovered rows and items |
| `color-surface-selected` | `#E7EAED` | `#2A3139` | Selected rows and items |
| `color-border` | `#E1E4E8` | `#2B3138` | Default borders and dividers |
| `color-border-strong` | `#C9CED4` | `#3A424B` | Emphasized dividers, borders of inputs, selects, checkboxes, and switches, drop target outlines, the today line |
| `color-text-primary` | `#16191D` | `#E8EBEE` | Main text |
| `color-text-secondary` | `#4C535C` | `#A8B0B9` | Supporting text |
| `color-text-muted` | `#6A727B` | `#858D96` | Meta text, placeholders, disabled labels |
| `color-text-inverse` | `#F5F6F7` | `#15181C` | Text on ink backgrounds |

### 2.2 Ink (primary actions)

| Token | Light | Dark | Use |
|---|---|---|---|
| `color-ink` | `#16191D` | `#E8EBEE` | Primary button background, selected nav marker, focus ring |
| `color-ink-hover` | `#2A2F35` | `#FFFFFF` | Primary button hover |
| `color-on-ink` | `#FFFFFF` | `#15181C` | Text and icons on ink |

### 2.3 Status flags

Each status has three tokens:

- `solid`: dots, card edges, and icons on surfaces.
- `text`: readable text in that color on surfaces, and icons and text inside a `subtle` background.
- `subtle`: tinted backgrounds for badges and banners. Never put a `solid` icon on a `subtle` background. Use the `text` token there.

| Status | Flag | Light solid | Light text | Light subtle | Dark solid | Dark text | Dark subtle |
|---|---|---|---|---|---|---|---|
| `planning` | White (preparing) | `#8A929B` | `#4C535C` | `#F1F3F5` | `#6B737D` | `#A8B0B9` | `#20252B` |
| `working` | Green (go) | `#1E9E5A` | `#146C3E` | `#E7F5EC` | `#3DBE7A` | `#6FD69E` | `#15291F` |
| `needs-you` | Yellow (caution) | `#BF8700` | `#7F5A00` | `#FCF4DC` | `#F2B81F` | `#F5CC5C` | `#2E2610` |
| `review` | Blue (let pass) | `#2E6BD9` | `#1F4FA8` | `#E8EFFC` | `#5B8DEF` | `#8DB0F5` | `#182338` |
| `ready` | Pit lane teal | `#0F97A0` | `#0A6A71` | `#E3F4F5` | `#2BB6BF` | `#6BD0D6` | `#11292C` |
| `done` | Checkered | `#16191D` | `#4C535C` | `#F1F3F5` | `#E8EBEE` | `#A8B0B9` | `#20252B` |
| `danger` | Red (stop) | `#D53B3B` | `#A32323` | `#FBEAEA` | `#EF5A5A` | `#F58A8A` | `#331A1B` |

Token names follow the pattern `color-status-<status>-<solid|text|subtle>`, for example `color-status-working-solid`.

### 2.4 Status to board state

| Card state | Status tokens | Icon | Extra |
|---|---|---|---|
| Backlog | none (neutral) | Dashed circle | No colored edge |
| Planning | `planning` | Outlined flag | |
| Working | `working` | Filled flag | Status dot pulses gently |
| Needs you | `needs-you` | Raised hand | Shows the reason, for example "Plan ready" or "Approval needed" |
| In review | `review` | Eye | |
| Ready to merge | `ready` | Merge arrows | |
| Merging | `ready` | Merge arrows | Progress bar in `ready-solid` |
| Done | `done` | Checkered flag | Card text uses secondary color |
| Asleep (any state) | Keeps its state color | Moon beside the state icon | Card text uses secondary color |

`danger` is not a card state. It is used for failures inside a state: failed CI, failed checks, stuck detection, errors, and bypass mode.

### 2.5 CI badges

| CI state | Tokens | Icon |
|---|---|---|
| Queued or running | Neutral text | Small spinner |
| Passed | `working` | Check |
| Failed | `danger` | Cross |
| Cancelled | Neutral muted | Slash |

### 2.6 Semantic feedback

| Token | Maps to | Use |
|---|---|---|
| `color-success-*` | `working` values | Success toasts and messages |
| `color-warning-*` | `needs-you` values | Warnings |
| `color-danger-*` | `danger` values | Errors and destructive actions |
| `color-info-*` | `review` values | Information notices |

### 2.7 Bypass mode

| Token | Light | Dark | Use |
|---|---|---|---|
| `color-bypass-bg` | `#B42323` | `#C83A3A` | Bypass banner background |
| `color-bypass-stripe` | `#9C1C1C` | `#A92E2E` | Diagonal stripe pattern on the banner |
| `color-bypass-text` | `#FFFFFF` | `#FFFFFF` | Banner text and icon |

The bypass banner uses a diagonal stripe pattern, so it can never be mistaken for an ordinary error message.

### 2.8 Diffs

| Token | Light | Dark |
|---|---|---|
| `color-diff-added-bg` | `#E7F5EC` | `#15291F` |
| `color-diff-added-line` | `#CDEBD8` | `#1D3A2B` |
| `color-diff-added-text` | `#146C3E` | `#6FD69E` |
| `color-diff-removed-bg` | `#FBEAEA` | `#331A1B` |
| `color-diff-removed-line` | `#F4D2D2` | `#4A2224` |
| `color-diff-removed-text` | `#A32323` | `#F58A8A` |

### 2.9 Contrast rules

- Body text on its surface: at least 4.5 to 1.
- Large text (18 px and up, or 14 px bold): at least 3 to 1.
- Icons, borders of inputs, status solids, and chart marks against their surface: at least 3 to 1.
- Status is never shown by color alone. Every status has an icon and a text label as well.

**Surfaces.** "Their surface" means any of `color-canvas`, `color-surface`, `color-surface-sunken`, `color-surface-raised`, `color-surface-hover`, and `color-surface-selected`. A token that can sit on a surface must pass on it.

**The design's colors are used exactly.** The values in this file are the ones from the Claude Design prototype, and the app uses them as they are. Some pairs are below the targets above (for example muted text on the selected surface, or a status solid on a hovered surface). They are listed, with their ratios, in `packages/tokens/src/contrast-exceptions.ts` (35 pairs at the last count). They stay until the design changes.

**Pairs checked in CI.** `check-contrast.ts` reads the values in `packages/tokens` and checks each of these. It fails on any pair that fails and is not in the exceptions list, and on any exception that now passes, so the list can only shrink:

| Foreground | Background | Minimum |
|---|---|---|
| `color-text-primary`, `color-text-secondary`, `color-text-muted` | Every surface | 4.5 to 1 |
| `color-status-<status>-text` | Every surface, and its own `subtle` | 4.5 to 1 |
| `color-status-<status>-solid` | Every surface | 3 to 1 |
| `color-border-strong` (control borders) | Every surface | 3 to 1 |
| `color-ink` (focus ring) | Every surface | 3 to 1 |
| `color-text-inverse`, `color-on-ink` | `color-ink`, and `color-ink-hover` for `color-on-ink` | 4.5 to 1 |
| `color-diff-*-text` | Its diff background and diff line | 4.5 to 1 |
| `color-bypass-text` | `color-bypass-bg` and `color-bypass-stripe` | 4.5 to 1 |
| `color-chart-1` to `color-chart-4`, `color-chart-limit` | `color-surface` and `color-canvas` | 3 to 1 |
| `color-chart-axis` | `color-surface` and `color-canvas` | 4.5 to 1 |

`color-border` is a soft divider and is not checked. `color-border-strong` is the border of every control in the design, so it is checked at 3 to 1 like a control border, and most of its pairs are in the exceptions list.

Every theme, including custom ones, is checked against these rules in CI.

---

## 3. Typography

### 3.1 Typefaces

| Token | Family | Use |
|---|---|---|
| `font-sans` | Atkinson Hyperlegible Next, then system UI fonts | All interface text |
| `font-mono` | Atkinson Hyperlegible Mono, then system monospace fonts | Code, file paths, branch names, commands, terminal |

**Why this pair:** Marshal is read at small sizes all day: branch names, IDs, file paths, logs. Atkinson Hyperlegible was designed for legibility, with clearly different shapes for characters that are easy to confuse, such as `l`, `1`, and `I`, or `0` and `O`. The sans and mono come from one family, so they sit well together while staying clearly different in role.

Mono is used only for real code and machine text. It is never used for labels or decoration.

### 3.2 Type scale

The base size is 14 px, which suits a dense working tool.

| Token | Size | Line height | Weight | Use |
|---|---|---|---|---|
| `text-caption` | 12 px | 16 px | 400 | Timestamps, small meta |
| `text-small` | 13 px | 18 px | 400 | Card meta, secondary lines, table cells |
| `text-body` | 14 px | 20 px | 400 | Default text |
| `text-body-strong` | 14 px | 20 px | 600 | Card titles, emphasized text |
| `text-subtitle` | 16 px | 22 px | 600 | Panel and section titles |
| `text-title` | 18 px | 24 px | 600 | Dialog titles, card detail title |
| `text-view-title` | 22 px | 28 px | 700 | View titles, used sparingly |
| `text-code` | 13 px | 20 px | 400 | Code blocks, paths, diffs, terminal |

### 3.3 Weights

| Token | Value |
|---|---|
| `weight-regular` | 400 |
| `weight-medium` | 500 |
| `weight-semibold` | 600 |
| `weight-bold` | 700 |

### 3.4 Rules

- Letter spacing stays at the font's default. No tracked-out labels.
- No all-caps text, and no `text-transform: uppercase`.
- Chat messages and long text have a maximum width of 72 characters (`size-measure`).

---

## 4. Spacing

A 4 px base scale.

| Token | Value |
|---|---|
| `space-0` | 0 |
| `space-0.5` | 2 px |
| `space-1` | 4 px |
| `space-2` | 8 px |
| `space-3` | 12 px |
| `space-4` | 16 px |
| `space-5` | 20 px |
| `space-6` | 24 px |
| `space-8` | 32 px |
| `space-10` | 40 px |
| `space-12` | 48 px |
| `space-16` | 64 px |

Common uses:

- Inside a card: `space-3`.
- Between cards in a column: `space-2`.
- Between columns: `space-3`.
- Panel padding: `space-4` or `space-5`.
- Between sections in a view: `space-6` or `space-8`.

---

## 5. Radius

Radius follows hierarchy. Small controls are tighter, large surfaces are softer. We do not use one radius for everything.

| Token | Value | Use |
|---|---|---|
| `radius-xs` | 3 px | Badges, checkboxes, tags, code spans |
| `radius-sm` | 5 px | Inputs, selects, agent avatars (rounded squares) |
| `radius-md` | 7 px | Buttons up to 32 px tall, cards, list rows with a background |
| `radius-lg` | 10 px | Buttons 36 px tall and every button on touch screens, panels, popovers, menus |
| `radius-xl` | 14 px | Dialogs, the command palette |
| `radius-full` | 9999 px | Status dots, person avatars, toggles |

---

## 6. Borders and elevation

Most surfaces are separated by borders, not shadows. Shadows are only for things that float above the page.

| Token | Value | Use |
|---|---|---|
| `border-width` | 1 px | Default |
| `border-width-strong` | 2 px | Focus rings, selected edges |
| `card-edge-width` | 3 px | Colored status edge on the left of a card |
| `elevation-0` | none | Cards, panels, columns |
| `elevation-1` | Light: `0 6px 16px -4px rgb(22 25 29 / 0.16)` with a 1 px border. Dark: `0 6px 16px -4px rgb(0 0 0 / 0.5)` with a 1 px border | Popovers, menus, tooltips |
| `elevation-2` | Light: `0 16px 40px -8px rgb(22 25 29 / 0.24)`. Dark: `0 16px 40px -8px rgb(0 0 0 / 0.6)` | Dialogs, command palette |
| `elevation-drag` | Light: `0 10px 24px -6px rgb(22 25 29 / 0.22)`. Dark: `0 10px 24px -6px rgb(0 0 0 / 0.55)` | A card being dragged |

### Focus ring

- A 2 px ring in `color-ink`, with a 2 px gap in the surface color.
- Shown only for keyboard focus (`:focus-visible`), always visible, never removed.

---

## 7. Motion

Motion is used to show what changed after an action, and for a very small number of status signals. It is never decoration.

| Token | Value | Use |
|---|---|---|
| `duration-instant` | 80 ms | Press states, toggles |
| `duration-fast` | 140 ms | Hover, small reveals, tooltips |
| `duration-base` | 200 ms | Menus, popovers, card moves |
| `duration-slow` | 280 ms | Panels and dialogs opening |
| `ease-standard` | `cubic-bezier(0.2, 0, 0, 1)` | Most movement |
| `ease-enter` | `cubic-bezier(0, 0, 0, 1)` | Things appearing |
| `ease-exit` | `cubic-bezier(0.4, 0, 1, 1)` | Things leaving |
| `pulse-working` | 1600 ms, opacity 0.45 to 1, ease in and out, repeating | The status dot of a working card, and the placeholder shapes of a skeleton while a part loads (`Skeleton` in `@marshal/ui`) |

Rules:

- With reduced motion turned on in the OS, all movement is replaced by instant changes, and the working pulse becomes a static dot.
- Only one thing pulses on screen per card: the working dot. Skeleton shapes pulse only while their content loads, and never on a card that already has its content.

---

## 8. Layout sizes

| Token | Value | Use |
|---|---|---|
| `size-sidebar` | 232 px | Expanded sidebar |
| `size-sidebar-collapsed` | 56 px | Collapsed sidebar |
| `size-header` | 48 px | View header: title, view switcher, filters, and actions |
| `size-column` | 288 px | Kanban column width (min 260 px, max 340 px) |
| `size-detail-min` | 480 px | Card detail panel minimum width |
| `size-detail-max` | 760 px | Card detail panel maximum width |
| `size-measure` | 72ch | Maximum width of chat and long text |
| `size-control-sm` | 28 px | Small button and input height |
| `size-control-md` | 32 px | Default button and input height |
| `size-control-lg` | 40 px | Large buttons, mobile touch targets |
| `size-touch-min` | 44 px | Minimum touch target on tablets and phones |
| `size-topbar` | 48 px | App top bar: project name, search, notices, and profile avatar. Same on every size. |
| `size-mobile-nav` | 56 px | Bottom navigation height on phones, plus the safe area |
| `size-chat-list` | 280 px | Chat list width on desktop |
| `size-avatar` | 28 px | Profile avatar in the top bar |
| `size-avatar-sm` | 20 px | Member avatars on board cards and done checklist items |
| `size-avatar-md` | 24 px | Members row, comments |

### Breakpoints

| Token | Width | Layout |
|---|---|---|
| `bp-sm` | 640 px | Below this is a phone: single pane, top bar and bottom navigation. From here up, tablet layouts: icon sidebar, panels as overlays. |
| `bp-md` | 900 px | Tablet in landscape and small laptops. Split view becomes available, with up to two panes. Layouts stay tablet layouts. |
| `bp-lg` | 1200 px | Desktop from here: full sidebar, board with detail panel, split view with up to three panes |
| `bp-xl` | 1600 px | Large screens: split view with up to four panes |

---

## 9. Layers

| Token | Value | Use |
|---|---|---|
| `z-base` | 0 | Page content |
| `z-sticky` | 10 | Sticky headers, column headers |
| `z-banner` | 50 | Bypass banner |
| `z-dropdown` | 100 | Menus, selects |
| `z-overlay` | 200 | Dialog backdrop |
| `z-dialog` | 300 | Dialogs |
| `z-palette` | 400 | Command palette |
| `z-toast` | 500 | Toasts |
| `z-tooltip` | 600 | Tooltips |

---

## 10. Icons

| Token | Value |
|---|---|
| `icon-sm` | 14 px, stroke 1.75 |
| `icon-md` | 16 px, stroke 1.5 (default) |
| `icon-lg` | 20 px, stroke 1.5 |

- Icon set: Lucide.
- Status flag icons are our own small SVG set in `packages/ui/icons/status`, drawn to match Lucide's stroke style. This includes the checkered flag, which Lucide does not have.
- Icons inherit `currentColor`. Only status icons take status colors.

---

## 11. Themes

- Themes are CSS variables on `:root`, with the dark theme under `[data-theme="dark"]`.
- The default follows the OS setting. Users can pick light, dark, or system.
- **Custom themes** override the neutral, ink, and status tokens. A custom theme must pass the contrast rules in section 2.9 before it can be saved.
- Tailwind reads these variables through its theme config, so classes such as `bg-surface` and `text-secondary` map to tokens.

### 11.1 CSS variables

```css
:root {
  /* neutrals */
  --color-canvas: #F5F6F7;
  --color-surface: #FFFFFF;
  --color-surface-sunken: #EEF0F2;
  --color-surface-raised: #FFFFFF;
  --color-surface-hover: #F0F2F4;
  --color-surface-selected: #E7EAED;
  --color-border: #E1E4E8;
  --color-border-strong: #C9CED4;
  --color-text-primary: #16191D;
  --color-text-secondary: #4C535C;
  --color-text-muted: #6A727B;
  --color-text-inverse: #F5F6F7;

  /* ink */
  --color-ink: #16191D;
  --color-ink-hover: #2A2F35;
  --color-on-ink: #FFFFFF;

  /* status flags */
  --color-status-planning-solid: #8A929B;
  --color-status-planning-text: #4C535C;
  --color-status-planning-subtle: #F1F3F5;
  --color-status-working-solid: #1E9E5A;
  --color-status-working-text: #146C3E;
  --color-status-working-subtle: #E7F5EC;
  --color-status-needs-you-solid: #BF8700;
  --color-status-needs-you-text: #7F5A00;
  --color-status-needs-you-subtle: #FCF4DC;
  --color-status-review-solid: #2E6BD9;
  --color-status-review-text: #1F4FA8;
  --color-status-review-subtle: #E8EFFC;
  --color-status-ready-solid: #0F97A0;
  --color-status-ready-text: #0A6A71;
  --color-status-ready-subtle: #E3F4F5;
  --color-status-done-solid: #16191D;
  --color-status-done-text: #4C535C;
  --color-status-done-subtle: #F1F3F5;
  --color-status-danger-solid: #D53B3B;
  --color-status-danger-text: #A32323;
  --color-status-danger-subtle: #FBEAEA;

  /* bypass */
  --color-bypass-bg: #B42323;
  --color-bypass-stripe: #9C1C1C;
  --color-bypass-text: #FFFFFF;

  /* diffs */
  --color-diff-added-bg: #E7F5EC;
  --color-diff-added-line: #CDEBD8;
  --color-diff-added-text: #146C3E;
  --color-diff-removed-bg: #FBEAEA;
  --color-diff-removed-line: #F4D2D2;
  --color-diff-removed-text: #A32323;

  /* type */
  --font-sans: "Atkinson Hyperlegible Next", ui-sans-serif, system-ui, sans-serif;
  --font-mono: "Atkinson Hyperlegible Mono", ui-monospace, "SF Mono", Menlo, Consolas, monospace;
}

[data-theme="dark"] {
  --color-canvas: #15181C;
  --color-surface: #1B1F24;
  --color-surface-sunken: #111417;
  --color-surface-raised: #22272D;
  --color-surface-hover: #242A31;
  --color-surface-selected: #2A3139;
  --color-border: #2B3138;
  --color-border-strong: #3A424B;
  --color-text-primary: #E8EBEE;
  --color-text-secondary: #A8B0B9;
  --color-text-muted: #858D96;
  --color-text-inverse: #15181C;

  --color-ink: #E8EBEE;
  --color-ink-hover: #FFFFFF;
  --color-on-ink: #15181C;

  --color-status-planning-solid: #6B737D;
  --color-status-planning-text: #A8B0B9;
  --color-status-planning-subtle: #20252B;
  --color-status-working-solid: #3DBE7A;
  --color-status-working-text: #6FD69E;
  --color-status-working-subtle: #15291F;
  --color-status-needs-you-solid: #F2B81F;
  --color-status-needs-you-text: #F5CC5C;
  --color-status-needs-you-subtle: #2E2610;
  --color-status-review-solid: #5B8DEF;
  --color-status-review-text: #8DB0F5;
  --color-status-review-subtle: #182338;
  --color-status-ready-solid: #2BB6BF;
  --color-status-ready-text: #6BD0D6;
  --color-status-ready-subtle: #11292C;
  --color-status-done-solid: #E8EBEE;
  --color-status-done-text: #A8B0B9;
  --color-status-done-subtle: #20252B;
  --color-status-danger-solid: #EF5A5A;
  --color-status-danger-text: #F58A8A;
  --color-status-danger-subtle: #331A1B;

  --color-bypass-bg: #C83A3A;
  --color-bypass-stripe: #A92E2E;
  --color-bypass-text: #FFFFFF;

  --color-diff-added-bg: #15291F;
  --color-diff-added-line: #1D3A2B;
  --color-diff-added-text: #6FD69E;
  --color-diff-removed-bg: #331A1B;
  --color-diff-removed-line: #4A2224;
  --color-diff-removed-text: #F58A8A;
}
```

Spacing, radius, motion, layout, and layer tokens are the same in both themes and live in the same package.

---

## 12. Charts

Charts follow the same rule as everything else: color is a signal. Most charts use neutral shades. Status colors appear only when a series is a status.

| Token | Light | Dark | Use |
|---|---|---|---|
| `color-chart-1` | `#16191D` | `#E8EBEE` | Main series |
| `color-chart-2` | `#6A727B` | `#A8B0B9` | Second series |
| `color-chart-3` | `#A7AEB6` | `#6B737D` | Third series |
| `color-chart-4` | `#C9CED4` | `#3A424B` | Fourth series |
| `color-chart-grid` | `#E1E4E8` | `#2B3138` | Grid lines |
| `color-chart-axis` | `#6A727B` | `#858D96` | Axis labels |
| `color-chart-limit` | `#D53B3B` | `#EF5A5A` | Limit lines, drawn dashed |

Rules:

- More than four series means the chart is too busy. Group the rest as "Other".
- Series are told apart by position, labels, and line style (solid or dashed) as well as shade, never by color alone.
- Series marks aim for 3 to 1 against the chart's surface. The design's third and fourth series are lighter than that in the light theme and darker in the dark theme; they are in the exceptions list, and labels and line style carry the meaning.
- Axis labels use `text-caption`. Values in tooltips use `text-small`.
- Bars use `radius-xs` on the top corners only.
- Charts have a text summary for screen readers, such as "Cards finished per day, last 14 days, highest on Tuesday with 12".

```css
:root {
  --color-chart-1: #16191D;
  --color-chart-2: #6A727B;
  --color-chart-3: #A7AEB6;
  --color-chart-4: #C9CED4;
  --color-chart-grid: #E1E4E8;
  --color-chart-axis: #6A727B;
  --color-chart-limit: #D53B3B;
}

[data-theme="dark"] {
  --color-chart-1: #E8EBEE;
  --color-chart-2: #A8B0B9;
  --color-chart-3: #6B737D;
  --color-chart-4: #3A424B;
  --color-chart-grid: #2B3138;
  --color-chart-axis: #858D96;
  --color-chart-limit: #EF5A5A;
}
```
