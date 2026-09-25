import { CountBubble, cx, Icon, IconButton, Kbd, NeedsBadge } from "@marshal/ui";
import { createMemo, Show } from "solid-js";
import { M } from "~/mock";
import { AvatarMenu } from "./AvatarMenu";
import { buildNotices } from "./notices/notice-list";
import { openPalette } from "./shell-actions";
import {
  currentProject,
  isDesktop,
  isPhone,
  isProject,
  modKey,
  pageTitle,
  totalNeeds,
} from "./shell-layout";
import { openPicker, toggleNotices, toggleTheme } from "./sidebar-actions";

const themeLabel = (): string =>
  M.S.resolvedTheme === "dark" ? "Switch to light theme" : "Switch to dark theme";

/** On phones the title is the button that opens the project picker. */
function PickerButton() {
  return (
    <button
      type="button"
      data-tour="projects-phone"
      onClick={openPicker}
      aria-haspopup="dialog"
      class="min-w-0 flex-1 flex items-center gap-1.5 h-11 px-1 border-none bg-transparent text-left"
    >
      <span class="min-w-0 truncate text-subtitle leading-5.5 font-semibold">{pageTitle()}</span>
      <Icon name="chevron-down" size={16} />
      <Show when={totalNeeds() > 0 && M.S.route.page !== "home"}>
        <NeedsBadge count={totalNeeds()} class="text-small!" />
      </Show>
    </button>
  );
}

/** Title and language of the current page. */
function PageTitle() {
  return (
    <div class="flex-1 min-w-0 flex items-baseline gap-2">
      <h1 class="m-0 text-subtitle leading-5.5 font-semibold whitespace-nowrap overflow-hidden text-ellipsis">
        {pageTitle()}
      </h1>
      <Show when={isProject()}>
        <span class="text-small leading-4.5 text-secondary whitespace-nowrap">
          {currentProject().lang}
        </span>
      </Show>
    </div>
  );
}

/** The search box opens the command palette; on tablets it is an icon. */
function SearchButton() {
  return (
    <button
      type="button"
      data-tour="search"
      onClick={openPalette}
      aria-label="Search"
      title="Search or run a command"
      class={cx(
        "flex items-center gap-2 h-8 px-2 rounded-sm border border-border bg-surface-sunken text-muted text-small text-left hover:border-border-strong",
        isDesktop() ? "w-[220px] justify-start" : "w-10 justify-center",
      )}
    >
      <Icon name="search" size={14} />
      <Show when={isDesktop()}>
        <span class="flex-1">Search</span>
        <span class="flex gap-0.5">
          <Kbd>{modKey()}</Kbd>
          <Kbd>K</Kbd>
        </span>
      </Show>
    </button>
  );
}

function NoticesButton() {
  const count = createMemo(() => buildNotices(M, isPhone()).length);
  return (
    <button
      type="button"
      data-tour="notices"
      onClick={toggleNotices}
      aria-label={`Notices, ${count()}`}
      aria-expanded={M.S.noticesOpen}
      title="Notices"
      class={cx(
        "relative size-9 flex-none inline-flex items-center justify-center border-none rounded-sm text-primary hover:bg-surface-hover",
        M.S.noticesOpen ? "bg-surface-selected" : "bg-transparent",
      )}
    >
      <Icon name="bell" size={18} />
      <Show when={count() > 0}>
        <CountBubble class="absolute top-0.5 right-0">{count()}</CountBubble>
      </Show>
    </button>
  );
}

/** The bar above every page: title or picker, search, theme, notices, and profile. */
export function TopBar() {
  return (
    <header
      class={cx(
        "h-12 flex-none flex items-center gap-2 pt-[env(safe-area-inset-top)] border-b border-border bg-surface relative z-bar",
        isPhone() ? "px-2" : "px-4",
      )}
    >
      <Show
        when={isPhone()}
        fallback={
          <>
            <PageTitle />
            <SearchButton />
          </>
        }
      >
        <PickerButton />
      </Show>
      <IconButton
        label={themeLabel()}
        title={themeLabel()}
        icon={M.S.resolvedTheme === "dark" ? "sun" : "moon"}
        size={36}
        iconSize={18}
        tone="default"
        onClick={toggleTheme}
      />
      <NoticesButton />
      <AvatarMenu />
    </header>
  );
}
