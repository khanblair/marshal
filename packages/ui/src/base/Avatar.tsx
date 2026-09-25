import { type JSX, Match, Switch, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import { cx } from "./cx";

/** Size in px, as in the design. */
export type AvatarSize = 22 | 28 | 64;

export interface AvatarProps extends Omit<JSX.HTMLAttributes<HTMLSpanElement>, "children"> {
  /** `person` (default): a circle with initials. `agent`: a rounded square with a bot icon. */
  kind?: "person" | "agent";
  /** Initials of a person, up to two letters. */
  initials?: string;
  /** 22 (default), 28, or 64. */
  size?: AvatarSize;
  /** A 2 px surface-colored border, so overlapping avatars stay apart (card members). */
  ring?: boolean;
  /** A 1 px strong border (header, profile, comment agent). */
  bordered?: boolean;
}

const PERSON_SIZES: Record<AvatarSize, string> = {
  22: "size-5.5 text-micro",
  28: "size-7 text-badge",
  64: "size-16 text-view-title",
};

const AGENT_SIZES: Record<AvatarSize, string> = {
  22: "size-5.5 rounded-sm-plus",
  28: "size-7 rounded-md",
  64: "size-16 rounded-lg",
};

const AGENT_ICON_PX: Record<AvatarSize, number> = { 22: 12, 28: 14, 64: 20 };
const DEFAULT_SIZE: AvatarSize = 22;

function frameClass(agent: boolean, ring: boolean, bordered: boolean): string | false {
  if (ring) {
    const outline = agent ? " outline-1 outline-border-strong -outline-offset-2" : "";
    return `border-2 border-surface${outline}`;
  }
  return bordered && "border border-border-strong";
}

/**
 * A person or the agent. People are circles with initials on the selected
 * fill; the agent is a rounded square with a bot icon on the sunken fill.
 * Header and comment avatars that use 12 px initials add `text-caption!`.
 */
export function Avatar(props: AvatarProps) {
  const [local, others] = splitProps(props, [
    "kind",
    "initials",
    "size",
    "ring",
    "bordered",
    "class",
  ]);
  const size = () => local.size ?? DEFAULT_SIZE;
  const agent = () => local.kind === "agent";
  return (
    <span
      {...others}
      class={cx(
        "inline-flex flex-none items-center justify-center",
        agent()
          ? `bg-surface-sunken text-secondary ${AGENT_SIZES[size()]}`
          : `rounded-full bg-surface-selected text-primary font-bold ${PERSON_SIZES[size()]}`,
        frameClass(agent(), !!local.ring, !!local.bordered),
        local.class,
      )}
    >
      <Switch fallback={local.initials}>
        <Match when={agent()}>
          <Icon name="bot" size={AGENT_ICON_PX[size()]} />
        </Match>
      </Switch>
    </span>
  );
}
