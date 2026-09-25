import { type JSX, splitProps } from "solid-js";
import { cx } from "../base/cx";
import type { LooseString } from "../base/types";
import { Icon } from "../icons/Icon";
import type { IconName } from "../icons/icon-names";

/** CI run states, as the CI table in store.js. */
export type CiState = "queued" | "running" | "passed" | "failed" | "cancelled";

const CI: Record<CiState, { icon: IconName; color: string }> = {
  queued: { icon: "spinner", color: "text-secondary" },
  running: { icon: "spinner", color: "text-secondary" },
  passed: { icon: "check", color: "text-status-working-text" },
  failed: { icon: "x", color: "text-status-danger-text" },
  cancelled: { icon: "slash", color: "text-muted" },
};

/** Icon and text color class for a CI state. Unknown states look queued. */
export function ciAppearance(status: string): { icon: IconName; color: string } {
  return Object.hasOwn(CI, status) ? CI[status as CiState] : CI.queued;
}

export interface CiStatusProps extends JSX.HTMLAttributes<HTMLSpanElement> {
  status: CiState | LooseString;
  /** Icon size in px. Default 14. */
  iconSize?: number;
}

const ICON_PX = 14;

/**
 * A CI state as its icon and bold colored text: a spinner while queued or
 * running, a check when passed, a cross when failed, a slash when cancelled.
 * The label is the children, such as `Passed` or `Main passed`; leave it out
 * for the icon alone.
 */
export function CiStatus(props: CiStatusProps) {
  const [local, others] = splitProps(props, ["status", "iconSize", "class", "children"]);
  const look = () => ciAppearance(local.status);
  return (
    <span
      {...others}
      class={cx("inline-flex items-center gap-1 font-semibold", look().color, local.class)}
    >
      <Icon name={look().icon} size={local.iconSize ?? ICON_PX} />
      {local.children}
    </span>
  );
}
