import { cx, Icon, type IconName, type ToneKey, toneIconText } from "@marshal/ui";
import { For } from "solid-js";
import type { ProviderTest, ProviderTestCheck } from "~/mock";

/** How one check of the last test reads: a flag and a color family, by what it found. */
const CHECK_STYLES: Record<ProviderTestCheck["state"], { icon: IconName; tone: ToneKey }> = {
  passed: { icon: "check", tone: "done" },
  failed: { icon: "x", tone: "danger" },
  warning: { icon: "triangle-alert", tone: "needs-you" },
};

/**
 * The one check a connection test names "Summary" (`integrations/test.go`'s `CheckSummary`). Its
 * message is the row's own sentence, which the row already shows above this list, and its fix is
 * the failed check's own, so drawing it here would only repeat them.
 */
const SUMMARY_CHECK = "Summary";

/**
 * What a connection's last test looked at, one line per check, as `docs/architecture.md` section 18
 * asks. One connection kind's test answers the same shape as another's, so the provider rows and the
 * integration rows draw the same list.
 */
export function TestChecks(props: { test: ProviderTest }) {
  const checks = () => props.test.checks.filter((check) => check.name !== SUMMARY_CHECK);
  return (
    <ul class="flex flex-col gap-0.5">
      <For each={checks()}>
        {(check) => {
          const style = () => CHECK_STYLES[check.state];
          return (
            <li class="flex items-start gap-2 text-small leading-4.5">
              <Icon name={style().icon} class={cx("mt-0.5 shrink-0", toneIconText[style().tone])} />
              <span class="text-secondary">
                <span class="text-primary">{check.name}</span> {check.message}
              </span>
            </li>
          );
        }}
      </For>
    </ul>
  );
}
