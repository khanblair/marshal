import { Avatar, Menu, MenuItem, Scrim } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M } from "~/mock";
import { closeMenu, openSettingsSection } from "./shell-actions";
import { isPhone } from "./shell-layout";

const MENU_NAME = "avatar";
const POPOVER = "absolute right-0 top-[42px] w-[240px] z-[200]";

interface AvatarItem {
  icon: string;
  label: string;
  run: () => void;
}

const ITEMS: readonly AvatarItem[] = [
  { icon: "user-round", label: "Profile", run: () => openSettingsSection("profile") },
  { icon: "settings", label: "Settings", run: () => openSettingsSection("general") },
  { icon: "keyboard", label: "Keyboard shortcuts", run: () => openSettingsSection("shortcuts") },
  { icon: "map", label: "Replay tour", run: () => M.startTour() },
];

const initials = (): string =>
  M.S.profile.initials ||
  M.S.profile.name
    .split(/\s+/)
    .map((word) => word[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();

/** The header's profile button and its menu: a popover, or a bottom sheet on phones. */
export function AvatarMenu() {
  const open = () => M.S.menu === MENU_NAME;
  return (
    <div class="relative flex-none">
      <button
        type="button"
        data-tour="avatar"
        onClick={() => M.set({ menu: open() ? null : MENU_NAME, noticesOpen: false })}
        aria-label="Profile and settings"
        aria-expanded={open()}
        class="size-9 inline-flex items-center justify-center border-none rounded-full bg-transparent p-0 hover:bg-surface-hover"
      >
        <Avatar
          size={28}
          bordered
          initials={initials()}
          src={M.S.profile.avatar ?? undefined}
          class="text-caption!"
        />
      </button>
      <Show when={open()}>
        <Scrim tone="clear" fixed class="z-[190]" onClick={closeMenu} />
        <Menu sheet={isPhone()} scrim={false} class={isPhone() ? undefined : POPOVER}>
          <div class="flex flex-col pt-2 px-2.5 pb-2.5 border-b border-border mb-1">
            <span class="font-semibold">{M.S.profile.name}</span>
            <span class="text-small text-secondary">{M.S.profile.tailnet}</span>
          </div>
          <For each={ITEMS}>
            {(item) => (
              <MenuItem size={36} icon={item.icon} onClick={item.run}>
                {item.label}
              </MenuItem>
            )}
          </For>
        </Menu>
      </Show>
    </div>
  );
}
