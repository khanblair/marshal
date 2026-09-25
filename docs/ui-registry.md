# UI registry

This document lists every component in `@marshal/ui` (`packages/ui`), so nothing gets built twice and every part stays consistent with the design.

**Rule:** before building a component, look here. If something close exists, use it or extend it. If two places in the design share a pattern, it becomes a component here, not a copy in a view (design-port.md, section 8). Screens (`HomeView`, `CardDetail`, `ChatThread`, the app shell, and so on) live in `apps/web` and compose these parts.

Every component supports light and dark themes, keyboard use, visible focus (the global `:focus-visible` ring), and reduced motion (handled globally in the base CSS). Every component takes `class` and passes other attributes (`aria-*`, `data-*`, `title`, `ref`, native events) to its root element.

---

## 1. How to use

```tsx
import { Button, Dialog, Field, Icon, Input, StatusLabel } from "@marshal/ui";

<Dialog width={560} phone={M.mobile} onClose={close} onSubmit={create} aria-labelledby="nc-title">
  <h2 id="nc-title" class="m-0 text-title leading-6 font-semibold">New card in {project.name}</h2>
  <Field label="Title">
    <Input data-autofocus value={draft.title} onInput={(e) => setTitle(e.currentTarget.value)} />
  </Field>
  <StatusLabel state={card.state}>{card.stateLabel}</StatusLabel>
  <div class="flex justify-end gap-2">
    <Button onClick={close}>Cancel</Button>
    <Button variant="primary" type="submit" disabled={!draft.title.trim()}>
      Create card
    </Button>
  </div>
</Dialog>;
```

Conventions:

- **Size props are the design's pixels.** The design says `height:28px`, so write `size={28}`. Icon sizes are pixels too: `<Icon name="pin" size={14} />`.
- **Classes add, they do not override.** A component sets only what the design sets; `class` adds layout such as `flex-1`, `w-full`, or `absolute right-0`. There is no class merging, so when two classes set the same property the CSS order decides. To override a class the component sets, use Tailwind's important suffix: `px-4!`, `text-caption!`. For example, `Button` centers its content; a stretched button the design aligns left, such as Add a card, takes `justify-start! px-2!`.
- **Use token classes only.** See design-port.md, section 3. For 12 px and 11 px text use `text-caption` and `text-badge`, never `text-[12px]`: the base CSS enlarges those two classes on phones, as the design does.
- **Borders:** `border` for all sides, `border-t` or `border-b` for one side. Never add `border-solid` next to a one-side border; with preflight off it shows the browser's default width on the other sides.
- **Colors from data** (a status color the store picked) go in `style`, for example `<Icon name={f.icon} style={{ color: f.iconColor }} />`. Everything else is a class.
- **Events:** native wrappers (`Input`, `TextArea`, `Select`, `Checkbox`) pass native events through, so `onInput={(e) => ... e.currentTarget.value}` ports unchanged. Custom controls report values: `onValueChange` (`SegmentedControl`, `Composer`) and `onCheckedChange` (`Switch`).
- **Icon names from data** are typed `IconNameInput`: any string, with editor hints for the known names. An unknown name draws an empty icon, as the prototype does.
- **Layout by viewport:** the design switches layouts from `M.S.vw`, not CSS breakpoints. Components that change on phones take a `phone` flag (`Dialog`, `ToastRegion`) or a `sheet` flag (`Menu`).
- **Escape:** keep the app shell's global Escape handler for every layer, as the design has it. `Menu` and `Dialog` also close on Escape when they have `onClose` and focus is inside them, and they stop that key so the global handler does not close a second layer. Focus is often outside (a menu opened from its trigger, a dialog without a `data-autofocus` control), so the global handler is still needed. Arrow keys in `Menu` likewise work once focus is inside it.
- **Touch:** `compact` sets `data-compact="1"`, which keeps a control small on touch screens (the base CSS makes other buttons 44 px high there).
- **Adding a component:** one PascalCase `.tsx` per component in the fitting folder, props interface `<Component>Props`, a test next to it, then run `pnpm gen` (it rewrites `src/index.ts`; never edit that file) and add a row here. Files whose name starts with `_` stay private.
- **Tailwind must scan this package.** `apps/web/src/styles/app.css` needs `@source "../../../../packages/ui/src";`, or none of these classes reach the app's CSS.

---

## 2. Icons (`src/icons`)

| Component | Purpose | Props | Design origin | Status |
|---|---|---|---|---|
| `Icon` | An icon from the design's set: Lucide 0.460 icons, the hand-drawn status flags, and `spinner`. Port of `m-icon`: same wrapper, svg attributes, and stroke formula | `name: IconNameInput`, `size?: number` (16), `style?: JSX.CSSProperties`, plus span attributes | store.js `MIcon`, every `<m-icon>` | Built |
| `StatusDot` | Round state marker in the state's solid color; pulses only for `working`. Port of `m-dot` | `state: StatusKey \| "danger" \| LooseString`, `size?: number` (8), `style?: JSX.CSSProperties` | store.js `MDot`; CardItem doing line, CardDetail Doing now, AgentsView activity | Built |
| `StatusIcon` | A state's flag in the state's solid color | `state: StatusKey \| LooseString`, `palette?: "card" \| "column"` ("card": backlog muted; "column": backlog in the strong border color), plus `Icon` props except `name` | BoardView column headers and phone tabs, CardItem state line, filter menu, search results | Built |

Icon names (134): `activity`, `archive`, `archive-restore`, `arrow-down`, `arrow-left`, `arrow-right`, `arrow-up`, `bell`, `book-open`, `bookmark`, `bot`, `calendar`, `calendar-check`, `check`, `check-check`, `chevron-down`, `chevron-left`, `chevron-right`, `circle-check`, `circle-dashed`, `circle-dollar-sign`, `circle-stop`, `circle-x`, `clock`, `columns-2`, `columns-3`, `copy`, `copy-plus`, `cpu`, `dot`, `ellipsis`, `ellipsis-vertical`, `eye`, `file-diff`, `file-pen`, `file-plus`, `file-search`, `file-text`, `file-warning`, `flask-conical`, `folder-cog`, `folder-git-2`, `folder-minus`, `folder-open`, `folder-plus`, `folder-x`, `gantt-chart`, `gauge`, `git-branch`, `git-commit-horizontal`, `git-fork`, `git-merge`, `git-pull-request`, `github`, `hand`, `history`, `home`, `house`, `image`, `info`, `key-round`, `keyboard`, `life-buoy`, `link`, `list`, `list-checks`, `list-filter`, `mail`, `map`, `maximize-2`, `message-circle`, `message-square`, `message-square-plus`, `messages-square`, `minimize-2`, `monitor`, `moon`, `package`, `palette`, `panel-left-close`, `panel-left-open`, `panel-right-open`, `paperclip`, `pause`, `pencil`, `pin`, `pin-off`, `play`, `plug`, `plus`, `repeat`, `route`, `scan-search`, `search`, `search-x`, `send`, `send-horizontal`, `server`, `settings`, `settings-2`, `shield-alert`, `slash`, `sliders-horizontal`, `smartphone`, `sparkles`, `square`, `square-check`, `square-kanban`, `square-terminal`, `sun`, `sunrise`, `sunset`, `tablet`, `terminal`, `terminal-square`, `trash-2`, `trello`, `triangle-alert`, `upload`, `user-cog`, `user-round`, `users`, `wrench`, `x`, `zap`; the flags `st-backlog`, `st-planning`, `st-working`, `st-done` (drawn from the CUSTOM table) and `st-needs` (hand), `st-review` (eye), `st-ready` and `st-merging` (git-merge); and `spinner`. `icon-names.test.ts` scans `design/` and fails when this list and the design drift apart.

---

## 3. Base components (`src/base`)

| Component | Purpose | Props | Design origin | Status |
|---|---|---|---|---|
| `Button` | Text button with optional leading icon and trailing key hint. Corners are 7 px up to 32 and 10 px at 36 and on touch screens, and the label never wraps | `variant?: "primary" \| "secondary" \| "quiet" \| "destructive"` ("secondary"), `size?: 28 \| 32 \| 36` (32), `tone?: "default" \| "danger"`, `icon?: IconNameInput`, `iconSize?: number` (14 at 28, else 16), `kbd?: string`, `compact?: boolean`, plus button attributes (`type` is `"button"` unless set) | New card, dialog Cancel and confirm, Approve and Review plan, plan Approve, Edit, Reject, settings Save and Remove, Remove project (destructive), Onboarding Back and Continue (36) | Built |
| `IconButton` | Square icon-only button, with the same corner radii as `Button` (24 px keeps 3 px) | `label: string` (sets `aria-label`), `icon: IconNameInput`, `size?: 24 \| 28 \| 32 \| 36 \| 44` (28), `iconSize?: number` (16, or 12 at 24), `variant?: "ghost" \| "outline" \| "primary"` ("ghost"), `tone?: "secondary" \| "muted" \| "default"` ("secondary"), `compact?: boolean` | Sidebar collapse, header theme and notices (36), card expand and close, More actions and calendar arrows (outline), send (primary), project and chat row menus (muted), phone back (44) | Built |
| `SegmentedControl` | Sunken track of options with the selected one raised; arrow keys move the selection | `options: readonly SegmentOption<T>[]` (`{ value: T; label: string; icon?: IconNameInput; title?: string; disabled?: boolean }`), `value: T`, `onValueChange: (value: T) => void`, `label: string`, `kind?: "radio" \| "tabs"` ("radio"), `size?: 24 \| 26 \| 30 \| 40` (26), `fill?: boolean`, `compact?: boolean`, `unselectedTone?: "secondary" \| "primary"`, `segmentClass?: string` | View tabs (tabs, 26 or 40, compact), chart range (compact), calendar Month and Week (tabs), Chat or Terminal (24), New project source (30, fill, primary), Onboarding theme (30, primary, `px-3!`) | Built |
| `Badge` | Small square-cornered label or count | `tone?: BadgeTone` (a `ToneKey`, or `"selected" \| "outline" \| "bypass"`; default "neutral"), `size?: 18 \| 20 \| 22 \| 24` (20), `icon?: IconNameInput` | Card state pill and plan status (22, with a `StatusIcon` child), provider key state (22), Bypass (20; 18 in Agents), package (outline, `font-mono`), Starter (18, outline), column and tab counts (selected) | Built |
| `NeedsBadge` | The needs-you count: a raised hand and a number | `count: number`, `size?: 20 \| 22` (20) | Sidebar Home and project rows (`px-1.25!`), phone header (`text-small!`), phone Go to sheet (22) | Built |
| `CountBubble` | Small round count, placed over an icon | `tone?: "ink" \| "needs-you"` ("ink"), children | Notices count, collapsed project needs count, phone nav Home | Built |
| `Tag` | Removable chip on a sunken fill | `onRemove: () => void`, `removeLabel: string`, `size?: 28 \| 32` (32), `icon?: IconNameInput`, children | Filter chips (32), pending attachments in the comment box (28) | Built |
| `Kbd` | Keyboard key hint | `tone?: "default" \| "muted" \| "strong" \| "key" \| "current"` ("default"), `size?: "sm" \| "md"` ("md") | Header search `⌘` `K`, filter `/`, search result keys (strong, `min-w-5`), button hints (current), shortcuts list (key) | Built |
| `Avatar` | A person (circle with initials) or the agent (rounded square with a bot) | `kind?: "person" \| "agent"` ("person"), `initials?: string`, `size?: 22 \| 28 \| 64` (22), `ring?: boolean`, `bordered?: boolean` | Card members (ring), member chips and picker, comments (28), header profile (28, bordered, `text-caption!`), profile and Onboarding (64, bordered) | Built |
| `Menu` | Floating menu panel (`role="menu"`); closes on Escape and outside presses; arrow keys move between items | `onClose?: () => void`, `trigger?: () => Element \| undefined`, `sheet?: boolean` (phone bottom sheet over a scrim), `scrim?: boolean` (true; false leaves the sheet without its dim scrim, as the avatar menu on phones), plus div attributes; place it with classes | Sidebar project menu, avatar menu, filter, saved views, and columns menus, card More actions, member picker, chat row menus | Built |
| `MenuItem` | One menu row | `icon?: IconNameInput`, `iconSize?: number`, `iconClass?: string`, `leading?: JSX.Element`, `danger?: boolean`, `hint?: JSX.Element`, `size?: 30 \| 32 \| 34 \| 36 \| 48` (32), `kind?: "item" \| "radio" \| "check" \| "plain"` ("item"), `checked?: boolean`, `current?: boolean` | Rename, Remove (danger), filter options (30, status-colored icon, count hint), saved views (30, radio), people (34, check, `Avatar` leading), avatar menu (36), phone sheet rows (48, plain) | Built |
| `MenuLabel` | Group heading in a menu | div attributes | Filter groups, People and Agent, search result groups | Built |
| `MenuSeparator` | 1 px line in a menu | hr attributes | Saved views menu | Built |
| `Input` | 32 px text field | `invalid?: boolean`, `mono?: boolean`, plus input attributes | Dialog, settings, and Onboarding fields; filter search (`px-7!`) and chat search (`pl-7! pr-2!`) | Built |
| `TextArea` | Multi-line field that resizes vertically | `invalid?: boolean`, `mono?: boolean`, plus textarea attributes | New card description, plan steps, role instructions, card note (mono), quick add | Built |
| `Select` | Native select with `selected` set on the matching option | `options: readonly (SelectOption \| string)[]` (`{ value: string; label?: string; disabled?: boolean }`), `value?: string`, `danger?: boolean`, plus select attributes | New card template, role, agent; swimlanes; pane view; card session settings (danger for bypass); settings selects | Built |
| `Checkbox` | Native checkbox in the ink accent | `size?: 16 \| 18` (18), `tone?: "ink" \| "danger"`, `align?: "center" \| "start"`, plus input attributes | Start right away, keep branches and memory, bypass acknowledgement (danger), columns menu (16), checklist items, lock bypass | Built |
| `Switch` | On and off switch (`role="switch"`) | `checked: boolean`, `onCheckedChange: (checked: boolean) => void`, `label: string` | Schedule rows | Built |
| `Field` | Label above a control, with a hint or an error | `label: JSX.Element`, `hint?: JSX.Element`, `error?: JSX.Element`, `compact?: boolean`, plus label attributes | Every labeled field; compact in card session settings | Built |
| `ChoiceCard` | Large radio option drawn as a card | `selected: boolean`, plus button attributes | Theme cards, first project source | Built |
| `EmptyState` | Nothing to show yet: icon, message, action | `icon?: IconNameInput`, `action?: JSX.Element`, `messageClass?: string`, children | Empty board, no chat open, no preview | Built |
| `NoResults` | Bordered notice when search or filters hide every card | `onClear: () => void`, `clearLabel?: string` ("Clear search"), children | Board and List "No cards match" | Built |
| `Callout` | Tinted status box in 13 px text | `tone?: "warning" \| "neutral"` ("warning"), `icon?: IconNameInput`, children | Possible duplicate card, weak model warning, repository detection (neutral) | Built |
| `ProgressBar` | Thin rounded progress bar or meter | `value: number`, `label: string`, `tone?: "ready" \| "neutral" \| "working" \| "needs-you"` ("neutral"), `size?: 4 \| 6` (6), `meter?: boolean` | Merge progress (4, ready), checklist progress, context meter (meter) | Built |
| `IconLabel` | Inline icon followed by text | `icon: IconNameInput`, `size?: number` (14), `gap?: 3 \| 4 \| 6` (4), `iconClass?: string`, children | Card footer counts, Pinned, Asleep; card branch, package, pull request, cost; calendar legend; chat list target | Built |
| `SortHeader` | Sticky table header, optionally a sort button | `label: string`, `sort?: SortDirection` (`"ascending" \| "descending" \| "none"`), `onSort?: () => void`, `alignEnd?: boolean`, `size?: 34 \| 36` (36), plus th attributes | Agents and List tables | Built |
| `TableShell` | The Agents and List table: full width, 13 px text on an 18 px line, separate borders; puts `header` cells in the one header row and its children in the body | `header: JSX.Element` (`SortHeader`s), `minWidth: number` (px, inline style), children (`TableRow`s), plus table attributes | Agents and List tables | Built |
| `TableRow` | A body row that opens something: focusable (`tabindex="0"`), pointer cursor, hover fill; the selected fill is a class so hover still shows | `highlight?: boolean` (the card the keyboard focus is on), plus tr attributes (`data-card`, `aria-label`, `onClick`, `onKeyDown`) | Agents and List rows | Built |
| `TableCell` | A body cell with the 1 px line under it; padding, width, and wrapping come from `class` | td attributes | Agents and List cells | Built |
| `PhoneList` | The list that replaces a table on phones: 16 px sides, 24 px below, no bullets | ul attributes | Agents and List phone lists | Built |
| `PhoneRowTitle` | First line of a phone list row: bold title, muted number | `title: JSX.Element`, `num: string`, `titleColor?: string` (a store color, applied inline) | Agents and List phone rows | Built |
| `PhoneRowMeta` | A wrapping line of small facts (12 px across, 2 px between lines); the text color is a class | span attributes | Agents and List phone rows | Built |
| `Toast` | Result of an action, on ink | `actionLabel?: string`, `onAction?: () => void`, `onDismiss: () => void`, `dismissLabel?: string` ("Dismiss"), children | App toasts, including Undo and Open | Built |
| `ToastRegion` | Live region that stacks toasts | `phone?: boolean` | App toast stack | Built |

---

## 4. Layout components (`src/layout`)

| Component | Purpose | Props | Design origin | Status |
|---|---|---|---|---|
| `Dialog` | Modal over a dim scrim: centered panel, or a bottom sheet on phones. Keeps Tab inside, focuses `[data-autofocus]` on open, gives focus back on close | `width?: 460 \| 520 \| 560` (460), `phone?: boolean`, `role?: "dialog" \| "alertdialog"` ("dialog"), `onClose?: () => void`, `onSubmit?: JSX.EventHandler<HTMLFormElement, SubmitEvent>` (makes it a form), plus attributes | Confirm (460), Remove project (520), New card and New project (560) | Built |
| `Sheet` | Phone bottom sheet with a grab handle and a heading | `title?: string`, `onClose?: () => void` (adds the scrim), `grabber?: boolean` | Phone Go to and More sheets | Built |
| `Scrim` | Full-cover layer behind an overlay | `tone?: "side" \| "sheet" \| "dialog" \| "clear"` ("dialog"), `fixed?: boolean`; set the layer with a class such as `z-scrim` | Sidebar overlay, tablet card backdrop, phone sheets, dialogs, search palette, click catchers for the avatar menu and notices | Built |
| `NavItem` | Main navigation row with the current-page bar | `icon: IconNameInput`, `label: string`, `current?: boolean`, `collapsed?: boolean`, `trailing?: JSX.Element` | Sidebar Home and Settings | Built |

---

## 5. Board components (`src/board`)

| Component | Purpose | Props | Design origin | Status |
|---|---|---|---|---|
| `StatusLabel` | A state as bold colored text with its flag | `state: StatusKey \| LooseString`, `iconSize?: number` (14), `withIcon?: boolean` (true), children | CardItem state line, chat card and links, Agents, List, Timeline on phones, Home awake rows (no icon) | Built |
| `CiStatus` | A CI state as its icon and bold colored text | `status: CiState \| LooseString`, `iconSize?: number` (14), children (optional label) | Sidebar project CI, CardItem, card header, List CI column, Home and All CI health | Built |

---

## 6. Card components (`src/card`)

| Component | Purpose | Props | Design origin | Status |
|---|---|---|---|---|
| `DiffStat` | Added and removed line counts in the diff colors | `added: number \| string`, `removed: number \| string` | Chat diff row, card diff summary and file rows (`gap-2!`) | Built |

---

## 7. Chat components (`src/chat`)

| Component | Purpose | Props | Design origin | Status |
|---|---|---|---|---|
| `Composer` | Message box with a borderless growing field and an ink send button. Enter sends, Shift and Enter adds a line | `value: string`, `onValueChange: (value: string) => void`, `onSend: () => void`, `placeholder?: string`, `label: string`, `sendSize?: 28 \| 32 \| 44` (32), `maxHeight?: 140 \| 160` (160), `textareaRef?: (el: HTMLTextAreaElement) => void` | Card chat (28 or 44, 140), Chats (32, 160) | Built |
| `JumpToLatest` | Floating pill that scrolls back to the newest message | `offset?: 10 \| 12` (10), `label?: string` ("Jump to latest"), plus button attributes | Card chat (10), Chats (12) | Built |

---

## 8. Dashboard components (`src/dashboard`)

| Component | Purpose | Props | Design origin | Status |
|---|---|---|---|---|
| `SummaryTile` | One number with a label that opens a filtered view | `value: JSX.Element`, `label: JSX.Element`, `tone?: "default" \| "needs-you" \| "danger"`, plus button attributes | Home tiles | Built |
| `FeedItem` | One activity row: colored icon, text, project, time | `icon: IconNameInput`, `iconColor?: string`, `project?: string`, `when: string`, `whenTitle?: string`, `onClick?: () => void`, children | Home Recent activity, Recent activity page | Built |
| `ShowMoreFooter` | Show all or Show less, then a quiet link to the full page | `total: number`, `limit?: number` (5), `expanded: boolean`, `onToggle: () => void`, `viewAllLabel: string`, `onViewAll: () => void` | Home sections: activity, coming up, agents awake, CI health | Built |

---

## 9. Settings components (`src/settings`)

| Component | Purpose | Props | Design origin | Status |
|---|---|---|---|---|
| `SettingsSection` | A settings page: 22 px heading, optional description and actions, then content | `title: JSX.Element`, `description?: JSX.Element`, `actions?: JSX.Element`, `onSubmit?: JSX.EventHandler<HTMLFormElement, SubmitEvent>` (makes it a form) | Every Settings section | Built |

---

## 10. Collaboration components (`src/collab`)

None yet. Checklists, comments, and members appear only in the card panel, so they are built there from `Avatar`, `Checkbox`, `ProgressBar`, `Tag`, `IconLabel`, and `Menu`.

---

## 11. Helpers

| Export | What it is |
|---|---|
| `cx(...classes)` | Joins class names, skipping empty values. No merging. |
| `LooseString` | Any string, with editor hints for the literals it is joined with. |
| `IconName`, `IconNameInput`, `iconNames`, `isIconName`, `resolveIcon`, `iconAliases`, `SPINNER_ICON` | The icon name list and how a name is drawn. |
| `lucideIcons`, `LucideIconName`, `statusGlyphs`, `StatusGlyphName` | The icon tables behind `Icon`. |
| `iconStrokeWidth(size)` | The prototype's stroke formula, as a string with two decimals. |
| `StatusKey`, `ToneKey`, `isStatusKey`, `statusTone`, `statusIcon` | Card states, their color families, and their flags (the STATUS table). |
| `toneSolidBg`, `toneSolidText`, `toneIconText`, `toneText`, `toneSubtleBg` | Class names per tone, for custom elements such as a card's left edge. |
| `CiState`, `ciAppearance(status)` | CI states and their icon and color class. |
| `trapTab`, `focusInitial`, `FOCUS_DELAY_MS` | The dialog focus helpers, for custom overlays such as the search palette. |
| `TableSort`, `sortDirection(current, key)`, `toggleSort(current, key)`, `compareSortValues(x, y)` | Sort state of a table (`{ k, dir }`, dir 1 or -1), the `aria-sort` value of a header, the state after a header click, and the prototype's ordering of two column values (numbers by value, text by code unit). |

---

## 12. Built in `apps/web`, not here

These appear once in the design, so they live with their screen and compose the parts above: the app shell (sidebar, header, view header, filter bar, bottom navigation, split panes, resize handle, notices panel, search palette), the dialog contents (confirm, New card, New project, Remove project), `CardItem`, board columns and swimlanes, `CardDetail` and its tabs (activity, checkpoints, diff, checklists, acceptance checks, preview and screenshots, notes, bypass banner, terminal), `ChatThread` (tool calls, plan, approval, card preview), the Home charts, the Timeline and Calendar grids, the Settings pages, Onboarding, and the Tour.

## 13. Removed from the old plan

The design does not have these, so they are not built: `Tooltip` (the design uses `title`), `Popover` (menus and panels cover it), `Skeleton`, `Spinner` (now `Icon name="spinner"`), `ErrorMessage` (errors are plain text in `Field`), `Tabs` (now `SegmentedControl kind="tabs"`), `ResponsiveTable` and `DataTable` (views switch between a table and a list themselves), `MarkdownContent`, `CodeBlock`, `TerminalView` (no xterm; the design's terminal is text lines), `SmellFindings`, `SmellFindingItem`, `DismissReasonDialog`, `ResourcePanel`, `TemplateEditor`, `SmellProfileEditor`, `TestConnectionButton`, `ConnectionTestResult`, `McpServerRow`, `SkillRow`, `Sparkline`, `MentionSuggest`, `EvidenceLink`, and `AddChecklistMenu`.

---

## 14. Changing this registry

- **Adding:** add the row in the same change that adds the component, with status Built once it has tests.
- **Changing a built component:** set status to "Needs changes" with a short note, then back to Built when done.
- **Removing:** delete the row in the same change that removes the component.
