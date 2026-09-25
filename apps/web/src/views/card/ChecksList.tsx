import { Button, Icon } from "@marshal/ui";
import { Index, Show } from "solid-js";
import { M } from "~/mock";
import { CHECK_LOOKS } from "./checks-model";

export interface ChecksListProps {
  cardId: number;
}

/** The acceptance checks Marshal runs by itself, with a Run checks button. */
export function ChecksList(props: ChecksListProps) {
  const checks = () => M.S.checks[props.cardId] ?? [];
  return (
    <>
      <h3 class="m-0 mt-2 text-subtitle leading-5.5 font-semibold">Acceptance checks</h3>
      <div class="flex items-center gap-2.5">
        <span class="flex-1 text-small text-secondary">
          Marshal runs these by itself. The agent can't finish this card until every check passes.
        </span>
        <Button size={28} icon="play" onClick={() => M.runChecks(props.cardId)}>
          Run checks
        </Button>
      </div>
      <ul class="m-0 p-0 list-none border border-border rounded-md overflow-hidden">
        <Index each={checks()}>
          {(check) => {
            const look = () => CHECK_LOOKS[check().st];
            return (
              <li class="flex items-center gap-2.5 py-2.5 px-3 border-b border-border">
                <span class={`inline-flex ${look().colorClass}`}>
                  <Icon name={look().icon} size={16} />
                </span>
                <span class="flex-1 min-w-0 flex flex-col gap-px">
                  <span class="font-semibold">{check().name}</span>
                  <Show when={check().cmd}>
                    <code class="font-mono text-caption text-secondary">{check().cmd}</code>
                  </Show>
                </span>
                <span class={`text-small font-semibold ${look().colorClass}`}>{look().label}</span>
              </li>
            );
          }}
        </Index>
      </ul>
    </>
  );
}
