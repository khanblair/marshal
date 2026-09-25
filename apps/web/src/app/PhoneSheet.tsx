import { MenuItem, NeedsBadge, Sheet } from "@marshal/ui";
import { Index, type JSX, Show } from "solid-js";
import { M } from "~/mock";
import { moreItems, pickerItems, type SheetItem } from "./phone-sheet-items";
import { closeMenu } from "./shell-actions";
import { isPhone } from "./shell-layout";

/** A row's trailing badge or hint, whichever it has. */
function hintOf(item: SheetItem): JSX.Element {
  if (item.badge) return <NeedsBadge count={item.badge} size={22} />;
  return item.hint ? <span class="text-small text-secondary">{item.hint}</span> : undefined;
}

/** The phone's project picker (Go to) and More sheets, opened by `M.S.menu`. */
export function PhoneSheet() {
  const picker = () => M.S.menu === "picker";
  return (
    <Show when={isPhone() && (picker() || M.S.menu === "more")}>
      <Sheet title={picker() ? "Go to" : "More"} onClose={closeMenu}>
        <Index each={picker() ? pickerItems() : moreItems()}>
          {(item) => (
            <MenuItem
              size={48}
              kind="plain"
              icon={item().icon}
              current={item().current}
              onClick={item().run}
              hint={hintOf(item())}
            >
              {item().label}
            </MenuItem>
          )}
        </Index>
      </Sheet>
    </Show>
  );
}
