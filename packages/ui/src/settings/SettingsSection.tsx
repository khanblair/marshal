import { type JSX, Show, splitProps } from "solid-js";
import { Dynamic } from "solid-js/web";
import { cx } from "../base/cx";

export interface SettingsSectionProps
  extends Omit<JSX.HTMLAttributes<HTMLElement>, "title" | "onSubmit"> {
  /** The 22 px section heading, such as `Provider keys`. */
  title: JSX.Element;
  /** Secondary text under the heading, pulled up 8 px as in the design. */
  description?: JSX.Element;
  /** Controls at the end of the heading row, such as a `New role` button. */
  actions?: JSX.Element;
  /** Makes the section a `<form>` with this submit handler (Profile, Project settings). */
  onSubmit?: JSX.EventHandler<HTMLFormElement, SubmitEvent>;
}

const HEADING = "m-0 text-view-title leading-7 font-bold";

/** One settings page: heading, optional description and actions, then content, 16 px apart. */
export function SettingsSection(props: SettingsSectionProps) {
  const [local, others] = splitProps(props, [
    "title",
    "description",
    "actions",
    "onSubmit",
    "class",
    "children",
  ]);
  return (
    <Dynamic
      component={(local.onSubmit ? "form" : "section") as "section"}
      onSubmit={local.onSubmit as JSX.EventHandler<HTMLElement, SubmitEvent> | undefined}
      {...others}
      class={cx("flex flex-col gap-4", local.class)}
    >
      <Show when={local.actions} fallback={<h2 class={HEADING}>{local.title}</h2>}>
        <div class="flex flex-wrap items-center gap-3">
          <h2 class={cx(HEADING, "flex-1")}>{local.title}</h2>
          {local.actions}
        </div>
      </Show>
      <Show when={local.description}>
        <p class="-mt-2 mb-0 text-secondary max-w-[72ch]">{local.description}</p>
      </Show>
      {local.children}
    </Dynamic>
  );
}
