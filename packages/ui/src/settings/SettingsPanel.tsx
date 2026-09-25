import { type JSX, splitProps } from "solid-js";
import { Dynamic } from "solid-js/web";
import { cx } from "../base/cx";

export interface SettingsPanelProps extends Omit<JSX.HTMLAttributes<HTMLDivElement>, "onSubmit"> {
  /** Rows stacked in a column and clipped to the rounded corners (provider, schedule, and role lists). */
  list?: boolean;
  /** Makes the panel a `<form>` with this submit handler (the role editor). */
  onSubmit?: JSX.EventHandler<HTMLFormElement, SubmitEvent>;
}

/**
 * A settings box: 1 px light border, 10 px corners, surface fill. Padding and
 * inner layout come from classes, such as `flex flex-col gap-3 py-3.5 px-4`.
 */
export function SettingsPanel(props: SettingsPanelProps) {
  const [local, others] = splitProps(props, ["list", "onSubmit", "class"]);
  return (
    <Dynamic
      component={(local.onSubmit ? "form" : "div") as "div"}
      onSubmit={local.onSubmit as JSX.EventHandler<HTMLElement, SubmitEvent> | undefined}
      {...others}
      class={cx(
        "border border-border rounded-lg bg-surface",
        local.list && "flex flex-col overflow-hidden",
        local.class,
      )}
    />
  );
}
