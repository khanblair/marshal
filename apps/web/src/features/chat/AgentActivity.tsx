import { For } from "solid-js";

export function AgentActivity(props: { label: string }) {
  return (
    <div
      role="status"
      aria-live="polite"
      class="flex items-center gap-2 py-2 text-small text-secondary"
    >
      <span aria-hidden="true" class="inline-flex items-center gap-1">
        <For each={[0, 160, 320]}>
          {(delay) => (
            <span
              class="size-1.5 rounded-full bg-current animate-pulse-working motion-reduce:animate-none"
              style={{ "animation-delay": `${delay}ms` }}
            />
          )}
        </For>
      </span>
      <span>{props.label}</span>
    </div>
  );
}
