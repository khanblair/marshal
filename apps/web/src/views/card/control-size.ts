import { M } from "~/mock";

/** Height in px of the panel's buttons: 44 on a phone for touch, 28 elsewhere. */
export const TOUCH_CONTROL_PX = 44;
export const COMPACT_CONTROL_PX = 28;

/** The panel's icon-button size for the current screen. Reactive: reads the viewport width. */
export const controlSize = (): typeof TOUCH_CONTROL_PX | typeof COMPACT_CONTROL_PX =>
  M.mobile ? TOUCH_CONTROL_PX : COMPACT_CONTROL_PX;
