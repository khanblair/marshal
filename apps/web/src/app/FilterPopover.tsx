import { cx, Menu } from "@marshal/ui";
import type { JSX } from "solid-js";
import { closeMenu } from "./shell-actions";
import { isPhone } from "./shell-layout";

export interface FilterPopoverProps {
  /** Which edge of the trigger the popover lines up with. */
  side: "left" | "right";
  children: JSX.Element;
}

/**
 * The floating panel of the filter bar's menus: a 280 px popover under its button, or
 * a bottom sheet on phones. Desktop menus close with Escape through the shell's global
 * handler, as in the design; only the phone sheet closes on an outside press.
 */
export function FilterPopover(props: FilterPopoverProps) {
  return (
    <Menu
      sheet={isPhone()}
      onClose={isPhone() ? closeMenu : undefined}
      class={
        isPhone()
          ? undefined
          : cx(
              "absolute top-[38px] w-[280px] max-h-[420px] overflow-auto z-menu",
              props.side === "left" ? "left-0" : "right-0",
            )
      }
    >
      {props.children}
    </Menu>
  );
}
