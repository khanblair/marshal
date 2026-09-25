# UI rules

This document says how Marshal's interface should look, read, and behave. Tokens (the values) are in `ui-tokens.md`. Components (the parts) are in `ui-registry.md`. This document is about how they come together.

The goal: a calm, fast, readable tool where a person can glance at the screen and know what is moving, what is finished, and what needs them.

---

## 1. Core rules

1. **Color is a signal.** Only status uses color. Everything else is neutral. See `ui-tokens.md` section 1.
2. **Status is never color alone.** Every status has an icon and a text label too.
3. **Show what is real.** The UI shows the daemon's state. It never guesses or pretends a change happened before the daemon confirms it, except for instant feedback on drags, which rolls back if the daemon refuses.
4. **Keyboard first.** Every action can be done without a mouse.
5. **Fast by default.** Nothing blocks the whole screen. Heavy parts load only when needed.
6. **Quiet motion.** Motion only shows what changed after an action, plus the working pulse. No decorative animation.
7. **Plain words.** Name things by what the user understands, not by how the system works.
8. **Every size, every feature.** Phone, tablet, and desktop all get the full product. See section 9.

---

## 2. App layout

### 2.1 Desktop shell

```
+------------+----------------------------------------------------------+
| Sidebar    | Top bar: project name, search, notices, profile avatar   |
|            +----------------------------------------------------------+
| Home       | View header: title, view switcher, filters, actions      |
|            +----------------------------------------------------------+
| Projects   |                                                          |
|  api     2 |                     Current view                         |
|  web       |          (dashboard, chats, agents, board, list,         |
|  mobile  1 |           timeline, calendar)        +-------------------+
|            |                                      | Card detail panel |
| + Project  |                                      | (opens on the     |
|            |                                      |  right, resizable)|
| Settings   |                                      |                   |
+------------+--------------------------------------+-------------------+
```

- **Sidebar:** Home, the project list, a "New project" action, and Settings. Each project shows badges for needs you (a number in the needs-you color), CI health, and awake agents. Each project row has a menu with Rename, Project settings, and Remove. The sidebar can collapse to icons.
- **Top bar:** on the left, the current project name (or "Home"). On the right, in this order: search (opens the command palette), notices, and the **profile avatar**. The avatar is always in the top right corner on every screen size. Clicking it opens Settings on the Profile page.
- **View header:** the view title, the view switcher, filters and saved views, and view actions such as "New card".
- **Card detail panel:** opens on the right when a card is selected. It can be resized between the min and max detail sizes, or expanded to take the full view.
- **Bypass banner:** when any open card uses bypass mode, a striped banner sits at the top of the card detail panel, and a small bypass marker shows on the card itself.

### 2.2 View switcher

- Sits in the view header as a small segmented control: Chat, Agents, Board, List, Timeline, Calendar. Home is reached from the sidebar.
- Keyboard: `Ctrl` or `Cmd` plus a number switches views, 1 to 6 in the order above.
- The last view is remembered per project.

### 2.3 Split view

- Split view opens from 900 px wide (`bp-md`) with up to two panes. It allows three from 1200 px (`bp-lg`) and four from 1600 px (`bp-xl`).
- Any pane can hold a view or a card.
- Panes are resized by dragging their dividers, and closed from their header.

---

## 3. Views

### 3.1 Home dashboard

Home is a dashboard across all projects. The overview comes first: summary tiles and charts at the top, then what needs the user, then the details.

```
+----------------+----------------+----------------+----------------+
| Needs you  4   | Working now 6  | Merged today 9 | Cost today     |
|                |                |                | $4.12 of $20   |
+----------------+----------------+----------------+----------------+
| Cards finished, last 14 days       | Cost by project, this month  |
| [bar chart]                        | [line chart with limit line] |
+------------------------------------+------------------------------+
| Needs you (4)                                                     |
|  api-gateway    Plan ready for review        Fix token refresh    |
|  mobile-app     Approval needed: pnpm add    Offline sync         |
+------------------------------------+------------------------------+
| Recent activity                    | Coming up today              |
|  Merged "Add rate limit" 4 min ago |  09:30 Standup (calendar)    |
|  CI failed on web-dashboard        |  18:00 Evening brief         |
|  Plan approved on "Offline sync"   |  Nightly issue sweep         |
+------------------------------------+------------------------------+
| Agents awake (5 of 8)              | CI health                    |
+------------------------------------+------------------------------+
```

- **Summary tiles come first:** needs you, working now, merged today, and cost today against the daily limit. The needs you tile is always the first tile, so the count of waiting items is still the first thing the user sees. Each tile opens a filtered view. Tiles show one number and one short label. No decorative icons or gradients.
- **Charts come next:** cards finished per day, and cost per project with the limit shown as a line. A range control switches between 7 days, 30 days, and 90 days. Charts follow the chart tokens in `ui-tokens.md`.
- **Needs you list** follows the charts: oldest first, grouped by project. Each row opens its card. When nothing needs the user, the section says so in one line.
- **Recent activity:** a live feed across projects: merges, CI results, approvals, plans, schedule runs, and briefs. Each item links to its card, chat, or job. It is virtualized and loads more as the user scrolls.
- **Coming up today:** calendar events, scheduled jobs, briefs, and cards due.
- **Agents awake:** each awake agent with its card, state, and a sleep action. Shows the awake limit.
- **CI health:** main branch state per project.
- **Tutorial:** the first-run tutorial guide starts here (see section 12).

Not every dashboard section is a card. Sections are separated by space and simple dividers, and only the summary tiles use a bordered box, so the page does not turn into a grid of identical boxes.

### 3.2 Board

- One board per project, with columns: Backlog, Planning, Working, Needs you, In review, Ready to merge, Done.
- Columns sit on `surface-sunken`. Cards sit on `surface` with a border and a colored left edge for status.
- Column headers show the name and the number of cards. Headers stay visible while scrolling.
- **Swimlanes** group cards into rows (by role, agent, package, or label). Each lane can collapse.
- **Filters** sit in the view header. Active filters show as removable chips. Saved views appear in a menu beside them.
- The Done column shows the last 20 cards, with "Show all" to see more.
- **Add a card** sits at the bottom of the Backlog, Planning, and Working columns, with a small button beside it to start from a template. Clicking it opens an inline field for the title. `Enter` adds the card, `Esc` cancels.
  - In Backlog, the card waits. In Planning, the agent starts in plan first mode. In Working, the agent starts right away. The field says which will happen, for example "Adds the card and starts the agent".
  - With swimlanes on, each lane has its own "Add a card", and the new card takes that lane's role, agent, or package.
  - It does not appear in Needs you, In review, Ready to merge, or Done, because cards reach those columns by themselves.
- Dragging a card shows it lifted with `elevation-drag`. Drop targets highlight with a border in `border-strong`. If the daemon refuses the move, the card returns to its place with a short message saying why.

### 3.3 Card

```
+---+--------------------------------------------------+
|   | Fix token refresh on login                        |
| s | [flag] Working   Worker   claude-sonnet   high    |
| t |                                                  |
| a | Doing now: running auth tests                    |
| t |                                                  |
| u | [branch] marshal/42-fix-token-refresh            |
| s | [ci] Passed    [check] 4/7   [comment] 3   [clip] 2 |
|   | $0.84   [moon] if asleep          (AB) [bot]      |
+---+--------------------------------------------------+
```

- **Left edge:** 3 px in the status solid color. Backlog cards have no colored edge.
- **Title:** `text-body-strong`, up to two lines, then cut off with an ellipsis.
- **State line:** status icon and label, role, model, thinking mode. Show only what is set.
- **Doing now:** one line from the agent, only while working.
- **Needs you reason:** replaces "doing now" when the card needs you, for example "Plan ready for review" or "Approval needed: run npm install".
- **Footer:** branch name in mono, CI badge, checklist progress ("4/7"), comment count, attachment count, cost, and the asleep marker if asleep. Pinned cards show a small pin. Counts that are zero are hidden.
- **Members:** avatars on the right of the footer. People are circles with initials. The agent is a rounded square with a bot icon. They are told apart by shape, not color. More than three people shows as "+2".
- Items in the footer are separate elements with space between them. We do not join them into one string with dots.
- **Done cards** use secondary text color and the checkered flag, so they fade back.

### 3.4 Card detail

- **Header:** title (editable), state, and quick actions: Start, Pause, Sleep, Pin, Fork, and a menu for more.
- **Settings row:** agent, role, model, thinking mode, and permission mode, each as a small select. Changes take effect on the next turn.
- **Members row:** below the title, showing the people and the card's agent, with a **+** button to add or remove people. The agent follows the Agent setting.
- **Tabs:** Chat, Activity, Diff, Checklists, Comments, Preview, Notes. Chat is the default. Checklists and Comments show a count when there is something new.
- **Chat and terminal toggle** sits at the top right of the Chat tab. Switching shows "Switching to terminal" while the session resumes, then continues.

### 3.5 Chat UI (inside a card)

- User messages sit on the right on `surface-sunken`. Agent messages sit on the left on the plain surface.
- **Tool calls** show as a compact, collapsed row: icon, short action ("Edited src/auth/token.ts", "Ran pnpm test"), and result. Clicking expands the details.
- **Diffs** in chat show as a small summary with a link to open the Diff tab.
- **Plans** (plan first mode) show as a clear block with steps, files to touch, and risks, followed by Approve plan, Edit plan, and Reject buttons.
- **Approvals** show inline with the exact action being asked for, for example the full command, and Approve and Deny buttons. Keyboard: `Enter` to approve when focused, `Esc` to deny.
- The message box stays at the bottom. `Enter` sends, `Shift` plus `Enter` adds a new line.
- New output streams in without jumping. If the user has scrolled up, show a "Jump to latest" button instead of scrolling for them.

### 3.6 Terminal view

- Takes the full width of the card detail panel, or the full view when expanded.
- Uses `font-mono` at `text-code` size, and the theme's surface colors.
- The terminal loads only when opened.

### 3.7 Chats view

A project can have **many chats**, but it always has **one board**. Chats are conversations about the project. Cards they create go onto the project's single board.

- **Layout on desktop:** a chat list on the left, the open chat on the right.
- **Chat list:** sorted by last activity. Each row shows the title, who the chat talks to (the Orchestrator, a role, or a card's agent), and when it was last active. A search field sits on top, and an "Archived" section sits at the bottom, collapsed.
- **New chat:** a "New chat" button at the top of the list. The user picks who to talk to. The Orchestrator is the default.
- **Titles:** a new chat gets a short title from its first message. The user can rename it at any time.
- **Row menu:** Rename, Archive (or Restore in the archived section), and Delete.
- **Inside a chat:** cards created from chat show as small card previews, which open the card when clicked. Board questions ("What is blocked?") answer with real card links.
- Each chat has its own lasting session, which follows the same sleep and wake rules as cards.

### 3.8 Agents view

- A table of every session: card, role, agent, model, thinking mode, permission mode, state, awake or asleep, current activity, and cost.
- Sortable by any column. Row actions: open, sleep, wake, pin, stop.

### 3.9 List view

- Cards as a sortable, filterable table.
- Columns can be shown or hidden. Row height stays compact.

### 3.10 Timeline view

- Cards as bars over time, colored by status, with dependency lines between them.
- Today is marked by a thin vertical line in `border-strong`.
- Dragging a bar changes its planned dates. Dependencies that would break show a warning before saving.

### 3.11 Calendar view

- Month and week layouts.
- Shows scheduled jobs, briefs, due cards, and synced calendar events, each with its own small icon.
- Clicking an item opens the job or card.

---

## 4. Interaction rules

### 4.1 Keyboard

| Keys | Action |
|---|---|
| `Cmd` or `Ctrl` + `K` | Command palette |
| `Cmd` or `Ctrl` + `1` to `6` | Switch views |
| `N` | New card |
| `/` | Focus search or filter |
| Arrow keys | Move between cards and rows |
| `Enter` | Open the selected card |
| `Esc` | Close panel, menu, or dialog |
| `A` | Approve the focused approval |
| `S` | Sleep the selected card |
| `P` | Pin or unpin the selected card |

Single-letter shortcuts only work when focus is not in a text field. All shortcuts are listed in the command palette and in Settings.

### 4.2 Command palette

- Opens over everything with `elevation-2` and `radius-xl`.
- Searches actions, projects, cards, and settings at once.
- Shows each action's shortcut on its right side.

### 4.3 Buttons and actions

- **Primary:** ink background. At most one primary button per area.
- **Secondary:** surface background with a border.
- **Quiet:** text only, for less important actions.
- **Destructive:** danger colors. Always confirmed if it cannot be undone.
- Buttons say exactly what they do, starting with a verb: "Start card", "Approve plan", "Merge", "Resume session".
- An action keeps the same name everywhere. The button "Merge" leads to a toast that says "Merged".
- Buttons never end with an arrow character.

### 4.4 Confirmations

- Confirm only actions that cannot be undone, or that are risky: deleting a card, closing a card with unmerged work, removing a project, deleting a chat, turning on bypass mode, raising a cost limit.
- The confirm dialog says what will happen in one sentence, and the confirm button repeats the action name, for example "Delete card".
- Turning on bypass needs the user to tick a box that says they understand the agent can run any command in the worktree without asking.

### 4.5 Forms

- Labels above fields, in sentence case.
- Field borders use `color-border-strong`, as in the design.
- Help text below the field, in `text-small` and secondary color.
- Errors show below the field in danger text, saying what is wrong and how to fix it.
- Save buttons are disabled until something changes.

---

## 5. Feedback

### 5.1 Loading

- Use skeletons shaped like the real content for views and cards.
- Use a small spinner only inside buttons and badges.
- Never block the whole app while loading one part.

### 5.2 Empty states

An empty screen is an invitation to act. Say what goes here and give the next step.

| Where | Message | Action |
|---|---|---|
| No projects | Add a repository to start running agents on it. | Add project |
| Empty board | This board has no cards yet. | New card |
| Nothing needs you | Nothing needs you right now. 3 agents are working. | Open board |
| No search results | No cards match "payment retry". | Clear search |

### 5.3 Errors

- Say what happened and how to fix it. Do not apologize, and do not be vague.
- Good: "Couldn't resume this session. Claude Code 1.9 is not installed. Install it, or start a new session from the handoff summary."
- Bad: "Oops! Something went wrong."
- Technical details (error codes, logs) go in an expandable "Details" section.

### 5.4 Toasts

- Appear at the bottom right, one line, for results of the user's own actions: "Card created", "Merged", "Session resumed".
- Disappear after 4 seconds, unless they contain an action such as "Undo".
- Toasts never carry information the user must see. That goes in notices.

### 5.5 Notices

- Notices are for things that happen without the user acting: needs you, CI failed, sleep warnings, cost warnings.
- They appear in a notice list in the sidebar, and go to other channels based on settings.
- Related notices group into one: "3 cards are idle and will sleep in 2 minutes", with actions for each.

### 5.6 Connection states

Marshal talks to a daemon over the network, so it has states the design never drew. Each one has one component in `@marshal/ui` and shows in one place only.

| Situation | What shows | Component |
|---|---|---|
| The daemon cannot be reached: the app has no data yet, or it has some and the daemon has not answered after several tries | A full screen: "Can't reach the daemon", Try again, and the time to the next automatic try. It goes away by itself when the daemon answers | `ConnectionLost` |
| The app has data and the live connection drops, and the daemon still answers or has not been tried yet | A thin bar at the top: "You're offline." The data stays on screen | `OfflineBanner` |
| The app is starting and has no data yet | A skeleton of the app's own layout (sidebar, top bar, and board columns), not a spinner | `AppSkeleton` in the web app, built from the skeleton components |
| One part of the app is loading | Skeletons in the space that part will fill | `Skeleton`, `SkeletonLines`, `SkeletonRow`, and `SkeletonCard`, inside one `SkeletonGroup` |
| The daemon does not know this device | A full screen to paste the access token | `SignIn` |
| A service is not set up | "GitHub is not connected." and a Connect button | `NotConnected` |
| One section failed to load | That section's message, Try again, and Details | `ErrorState` |

Rules:

- **A dropped connection shows the banner first; only a daemon that stays silent shows a full screen.** While the app is reconnecting (the live connection dropped), the banner shows and the data stays where it is. When the checks show that the daemon does not answer (after three failed tries, or fifteen seconds), the app shows the full screen "Can't reach the daemon" even though it has data, and returns to the app by itself when the daemon answers. The banner says changes cannot be made until Marshal reconnects. It never says they are queued, because there are no queued offline actions.
- **Skeletons are for a part, never for the whole app**, with one exception: before the app has any data at all, it draws its own layout as placeholders (`AppSkeleton`), so the frame does not jump when the data arrives. They have the same box as the real content (padding, border, and corners), so nothing shifts when it arrives. The container being loaded has `aria-busy="true"`, and one `SkeletonGroup` around the shapes says "Loading" once, so a screen reader does not read twenty empty blocks. The soft pulse uses the pulse token and stops under reduced motion like every other animation.
- **`NotConnected` is only for a service that is not set up**, and it never shows sample rows, cards, or numbers (backend-checklist 2.3, rule 4). Say in one sentence what connecting gives you.
- **`ErrorState` is for one section, not the app.** An app that cannot reach the daemon shows `ConnectionLost`. The message says what happened and how to fix it (5.3).
- **None of them shows a stack trace**, a raw error, or a file path in its message. Technical text goes inside the expandable Details, written short by the caller: an error code and a reason.
- **Screen readers hear the message once.** The message of `ConnectionLost` is an alert when it appears, the banner is a polite status, and the "Trying again in 4 s" line is kept out of those live regions so it is not read every second. `ErrorState` is plain text and not an alert, because one hiccup can fail several sections at once and each alert would interrupt.
- **`SignIn` keeps the token in its field.** The token is not shown as text, put in an attribute, or logged, and it is trimmed before it is sent. The error under the field is a plain sentence from the caller.

---

## 6. Writing rules

- **Sentence case everywhere:** headings, buttons, labels, menu items, tabs, toasts, and errors.
- **No all-caps words**, and no `text-transform: uppercase`. For emphasis, use weight or placement.
- **No small labels floating above headings** just for decoration.
- **Plain words:** "Needs you" not "Pending human intervention". "Sleep" not "Suspend process".
- **Short:** a label does one job. Cut filler words.
- **Active voice:** "The agent opened a pull request", not "A pull request was opened".
- **Numbers and time:** use the user's locale. Show relative times for recent events ("4 min ago") and full dates on hover. Costs show two decimals: "$0.84".
- **Names of things stay the same everywhere.** A card is always a card, never a task in one place and a ticket in another.
- **No joining meta items with dots or dashes** in one string. Lay them out as separate items.

---

## 7. Accessibility

- All text and icons meet the contrast rules in `ui-tokens.md` section 2.9.
- Every interactive element can be reached and used with the keyboard, in a logical order.
- Focus is always visible, using the focus ring token.
- All icons that carry meaning have text labels, either visible or for screen readers.
- Status changes on the board are announced to screen readers politely, grouped, without flooding.
- Dialogs trap focus and return it to where it was when closed.
- Motion follows the OS reduced motion setting.
- Touch targets on mobile are at least 44 px.

---

## 8. Performance rules

- Virtualize any list that can pass 100 items: board columns, activity feeds, chat history, logs, diffs, and tables.
- Only the open card subscribes to full output. Other cards get state and summary updates only.
- The terminal, syntax highlighting, and the timeline load lazily.
- Keep the first view interactive in under one second on a normal laptop.
- Never re-render a whole board for one card's change.

---

## 9. Responsive layouts

Marshal must work fully on desktop, tablet, and phone. This is essential for remote control: a user away from their desk must be able to see, approve, and steer everything from a phone.

**Rule:** every view and every action works at every size. Layouts adapt, features do not disappear.

### 9.1 Sizes

| Size | Width | Layout |
|---|---|---|
| Phone | Under 640 px | One pane at a time. Top bar and bottom navigation. |
| Tablet | 640 to 1199 px | Collapsed sidebar. Panels open as overlays. |
| Desktop | 1200 px and up | Full sidebar, detail panel beside the view, split view on large screens. |

### 9.2 How each part adapts

| Part | Desktop | Tablet | Phone |
|---|---|---|---|
| Navigation | Full sidebar | Icon sidebar, expands as an overlay | Bottom navigation: Home, Board, Chats, Agents, More |
| Top bar | Project name, search, notices, avatar | Same | Project name, notices, avatar. Search moves into More. |
| Home dashboard | Two-column sections | Two-column sections, narrower | One column: tiles in a two-by-two grid first, then charts, then needs you |
| Board | All columns side by side | Horizontal scroll, about two columns visible | One column at a time, with column tabs and swipe |
| Card detail | Side panel | Overlay panel over the board | Full screen, with tabs as a scrollable row |
| Chats | List and chat side by side | List and chat side by side, list narrower | List screen, then chat screen |
| Agents and list views | Full tables | Tables with fewer columns | Each row becomes a stacked item |
| Timeline | Horizontal time bars | Horizontal time bars, scrollable | Vertical day-by-day list with dependencies shown as "waits for" links |
| Calendar | Month and week | Month and week | Agenda list, with a month picker |
| Terminal | In the detail panel | Full overlay | Full screen, with a key bar for Esc, Tab, Ctrl, and arrow keys |
| Split view | Up to three panes, four from 1600 px | Up to two panes from 900 px wide, one pane below that | Not used. One pane at a time. |
| Command palette | Centered dialog | Centered dialog | Full screen sheet |
| Dialogs | Centered | Centered | Bottom sheets |

### 9.3 Touch and visibility

- Touch targets are at least 44 px on tablets and phones.
- Drag and drop has a touch-friendly option on every size: a "Move to" action in the card menu.
- Approvals, plan review, merges, sleep and wake, and bypass controls are always reachable within two taps from the notice or the card.
- Text never drops below 13 px on phones. Nothing is hidden only because the screen is small. If something does not fit, it moves into a menu or a scrollable row.
- The layout respects safe areas on phones with notches and home bars.
- Pages never scroll sideways, except inside the board and timeline, which are built to scroll.

### 9.4 Testing

Every view is checked at three sizes in end-to-end tests: a phone (390 by 844), a tablet (820 by 1180), and a desktop (1440 by 900), in both themes.

## 10. Do and don't

| Do | Don't |
|---|---|
| Use status colors only for status | Use a status color to decorate a button or heading |
| Pair every color with an icon and label | Rely on color alone |
| Use borders to separate surfaces | Put the same soft shadow under every card |
| Use ink for the one primary action | Make every button colorful |
| Keep done cards quiet | Make finished work as loud as active work |
| Write "Start card" | Write "Submit" or "Go" |
| Say how to fix an error | Say "Something went wrong" |
| Use sentence case | Write labels in all capital letters |
| Use mono only for code and machine text | Use mono for labels to look technical |
| Animate what changed after an action | Animate things on their own to look lively |

---

## 11. Projects and chats management

### 11.1 Projects

| Action | Where | Behavior |
|---|---|---|
| **Create** | "New project" in the sidebar, the command palette, or onboarding | A dialog asks for the repository folder (or a GitHub repo to clone), the name, and the default branch. Monorepos are detected automatically. |
| **Rename** | Project row menu, or click the project name in project settings | Inline edit. `Enter` saves, `Esc` cancels. The name must be unique. |
| **Edit** | Project settings | Default branch, integrations, limits, role overrides, and memory vault folder |
| **Remove** | Project row menu or project settings | See below |

**Removing a project:**

- Removes it from Marshal only. **The repository on disk is never deleted.**
- The confirm dialog lists what will happen: running sessions stop, worktrees are cleaned up, and cards and chats are deleted from Marshal.
- If any card has unmerged work, the dialog says how many and offers to keep those branches.
- The user can choose to keep the project's memory folder in the vault.
- The confirm button says "Remove project".

### 11.2 Chats

| Action | Behavior |
|---|---|
| **Create** | "New chat", pick who to talk to, start typing |
| **Rename** | Inline edit from the row menu, or by clicking the title in the open chat |
| **Archive** | Moves the chat to the archived section. Its session sleeps. Nothing is deleted. Undo is offered in a toast. |
| **Restore** | Moves an archived chat back to the list |
| **Delete** | Confirmed. Removes the chat and its history. Cards it created stay on the board. |

### 11.3 Inline rename rules

- Rename happens in place, not in a separate dialog.
- The field selects the whole name when it opens.
- `Enter` or clicking away saves. `Esc` cancels.
- Empty names are not allowed. The field shows what is wrong.

---

## 12. Onboarding and tutorial

### 12.1 Onboarding

Shown on first launch. Four screens, with a step indicator ("Step 2 of 4"), Back, Continue, and **Skip** on every screen. Skipped steps can be finished later from Settings.

| Step | Title | Content |
|---|---|---|
| 1 | Welcome to Marshal | One sentence on what Marshal does, and a simple picture of a board with cards moving through the flag colors |
| 2 | Connect your agents | Detected CLI agents with their versions, and fields to add API keys for providers. At least one agent or key is suggested, not required. |
| 3 | Add your first project | Pick a repository folder, or clone from GitHub. Option to use a sample project instead, to try Marshal safely. |
| 4 | Stay in control from anywhere | Pair a phone over Tailscale, and connect Telegram or Discord for notices. Both optional. |

- Onboarding works at every screen size.
- On the last screen, "Open Marshal" goes to the Home dashboard, where the tutorial starts.

### 12.2 Tutorial guide

A short guided tour on the Home dashboard that shows what the main controls do.

- **When:** after onboarding, the first time Home opens. It can be replayed any time from the profile menu or from Settings, under Help.
- **How it looks:** the page dims slightly, and one control at a time is highlighted with a small popover beside it. The popover has a title, one or two sentences, a step count, Back, Next, and **Skip tour**.
- **Steps:**

| Step | Highlights | Explains |
|---|---|---|
| 1 | Summary tiles and charts | How your agents and costs are doing |
| 2 | Needs you | Where things waiting on you appear |
| 3 | Recent activity | What happened across all projects |
| 4 | Project list | Each project has one board. Badges show what needs you. |
| 5 | New project | How to add a repository |
| 6 | View switcher | Moving between board, chats, agents, and other views |
| 7 | Search | The command palette and shortcuts |
| 8 | Notices | Alerts that happen while you are away |
| 9 | Profile avatar | Your profile and settings |

- The tour can be skipped at any step. Skipping asks nothing further.
- It works with the keyboard: `Enter` for next, `Esc` to skip.
- On phones, the popover becomes a small sheet at the bottom, and the highlighted control is scrolled into view.
- With reduced motion, highlights change instantly.

---

## 13. Profile

- Opened by clicking the **profile avatar** in the top right corner. It goes to Settings, on the Profile page.
- **Profile page contains:** name, avatar (image or initials), email (optional, used for briefs by email), time zone (used for briefs and schedules), paired devices with "Remove device", and the tailnet identity.
- In team mode, it also shows the user's role in each project.
- The avatar shows the user's image, or their initials on a neutral background. It never uses a status color.

---

## 14. Code smell findings

- Findings live in the card's **Checklists** tab, in a "Code smells" section after Marshal's acceptance checks (see section 16.1). The card itself shows a small count only when there are blocking findings, as part of the needs you reason or checks state.
- Findings are grouped by severity: blocking first, then warnings, then info. Info is collapsed by default.
- **Each finding shows:** the file and line (a link that opens the diff at that line), the smell's name and family, one sentence on why it matters, and a suggested refactoring.
- **Actions:** "Ask agent to fix" and "Dismiss". Dismiss opens a small dialog that asks for a reason. The reason is required.
- **Tone:** findings describe the code, not the agent or the person. Say "This function has 7 parameters. Group related ones into an options object." Do not say "Bad code" or "The agent made a mistake".
- Severity uses the existing status tokens: blocking uses `danger`, warnings use `needs-you`, and info is neutral. Each has an icon and a label as well as color.
- On phones, each finding is a stacked item, and the file link opens the diff full screen.
- The smell profile editor lives in project settings. It shows each check with its threshold and severity, grouped by family and by language, with "Reset to defaults".

---

## 15. Testing connections

Every outside connection has a **Test connection** button, so users can check it works without waiting for a real event.

- **Where:** on each integration in Settings (GitHub, Trello, Google Calendar, Gmail, Telegram, Discord, Obsidian vault, and Tailscale), on each provider API key, on each MCP server, and in onboarding steps 2 and 4.
- **While testing:** the button shows a small spinner and says "Testing". Nothing else on the screen is blocked.
- **Result:** shown right under the connection, with a time ("Tested 2 min ago"):
  - **Passed:** a check icon and a short list of what was checked, for example "Signed in as ada", "Can read issues and pull requests", "Webhook reached Marshal".
  - **Failed:** a cross icon, which check failed, why, and how to fix it, for example "Marshal can't read pull requests. Add the pull requests permission to the GitHub App, then test again." A "Details" section holds the technical error.
  - **Partly working:** a warning icon when sign-in works but something is missing, such as a permission or the webhook.
- **Colors:** passed uses `working`, partly working uses `needs-you`, failed uses `danger`, each with an icon and a label.
- **Test messages:** tests that send something visible say so on the button, for example "Send test message" for Telegram and Discord.
- **Cooldown:** after a test, the button waits a few seconds before it can run again, to respect the service's rate limits.
- Connections are also tested automatically right after they are added, and the last result shows on the integration's status.

---

## 16. Checklists, comments, and members

### 16.1 Checklists tab

- Shows every checklist on the card, in order, then Marshal's automatic acceptance checks below them, under a heading that says they are run by Marshal. Code smell findings sit in the same tab, after the acceptance checks.
- **Each checklist** has its name (renamed inline), a progress bar with a count ("4 of 7"), and a menu with "Hide checked items", "Required to finish", "Only people can tick", and Delete.
- **"Add an item"** sits at the bottom of each checklist, as an inline field. `Enter` adds the item and keeps the field open for the next one.
- **Each done item** shows who completed it: the person's avatar, or the agent's rounded square. Agent ticks have an "Evidence" link to the test run, check, or commit.
- **Required checklists** show a small "Required to finish" label beside the name. When they block the merge queue, the card's needs you reason says how many items are left.
- **"Add a checklist"** sits at the top of the tab, with choices for Sub-tasks, Definition of done, Acceptance criteria, or a custom name.
- Deleting a checklist is confirmed if it has ticked items.

### 16.2 Comments tab

- Comments are listed oldest first, with the composer at the bottom.
- **Each comment** shows the author's avatar and name, the time, the text, attachments, and link chips. The agent's comments use the agent avatar.
- **"Agent read this"** shows in small, secondary text under a comment once the agent has read it.
- **Attachments:** images preview inline and open larger on click. Other files show as a row with the file name, type, and size, with a download action.
- **Links:** pasted links become chips showing the site name and the link text. Chips never show fetched previews.
- **Composer:** a text field with buttons to attach a file or image and to add a link. Typing `@` suggests members and the agent. The field says, in placeholder text, that mentioning @agent or asking a question gets a reply.
- People can edit and delete their own comments. Deleted comments are removed, not shown as "deleted".

### 16.3 Members

- The **+** button opens a small picker with search, listing people in the project. Ticking a person adds them. Unticking removes them.
- The agent always shows first in the members row, and cannot be removed there. Its menu says "Change agent in settings", which jumps to the Agent setting.
- Avatars follow the shape rule: circle for people, rounded square for the agent. Never tell them apart by color.

### 16.4 On phones and tablets

- **Checklists:** full width, one checklist after another. Items have 44 px touch rows. "Add an item" stays at the bottom of each checklist.
- **Comments:** the composer is fixed to the bottom above the safe area, with attach and link buttons. Attaching can use the phone's camera or files. Images open full screen.
- **Members:** the picker opens as a bottom sheet.
- **Board cards:** counts and avatars stay visible. If the footer does not fit, counts move to a second line. Nothing is hidden.
- **Add a card** sits at the bottom of the visible column. The template button stays beside it.
