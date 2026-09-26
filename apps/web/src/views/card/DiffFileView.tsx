import { Button, DiffStat, Icon } from "@marshal/ui";
import { Index, Show } from "solid-js";
import type { DiffFile } from "~/mock";
import { hunkViews } from "./diff-model";

export interface DiffFileViewProps {
  file: DiffFile;
  open: boolean;
  loaded: boolean;
  onToggle: () => void;
  onLoad: () => void;
}

function Hunks(props: { file: DiffFile }) {
  return (
    <div class="overflow-x-auto border-t border-border">
      <Index each={hunkViews(props.file)}>
        {(hunk) => (
          <>
            <div class="py-0.5 px-2.5 bg-surface-sunken text-secondary font-mono text-caption leading-5 whitespace-pre">
              {hunk().header}
            </div>
            <Index each={hunk().lines}>
              {(line) => (
                <div class={`flex min-w-max font-mono text-small leading-5 ${line().bgClass}`}>
                  <span
                    class={`flex-none w-11 pr-2 text-right text-muted select-none ${line().gutterClass}`}
                  >
                    {line().n}
                  </span>
                  <span class={`flex-none w-4.5 text-center select-none ${line().textClass}`}>
                    {line().sign}
                  </span>
                  <span class={`whitespace-pre pr-3 ${line().textClass}`}>{line().text}</span>
                </div>
              )}
            </Index>
          </>
        )}
      </Index>
    </div>
  );
}

/** One changed file: a header that expands to its hunks, or to a Load diff notice when it is large. */
export function DiffFileView(props: DiffFileViewProps) {
  const large = () => !!props.file.large;
  return (
    <div class="border border-border rounded-md overflow-hidden">
      <button
        type="button"
        onClick={() => props.onToggle()}
        aria-expanded={props.open}
        class="w-full flex items-center gap-2 min-h-9 px-2.5 border-none bg-surface-sunken text-left hover:bg-surface-hover"
      >
        <Icon name={props.open ? "chevron-down" : "chevron-right"} size={14} />
        <span class="flex-1 min-w-0 overflow-hidden text-ellipsis whitespace-nowrap font-mono text-small">
          {props.file.path}
        </span>
        <DiffStat added={props.file.add} removed={props.file.del} class="gap-2!" />
      </button>
      <Show when={props.open && large() && !props.loaded}>
        <div class="flex flex-wrap items-center gap-2.5 p-3 border-t border-border text-small text-secondary">
          <Icon name="file-warning" size={14} />
          <span class="flex-1">
            Large file: {props.file.add.toLocaleString()} changed lines. It stays collapsed to keep
            the panel fast.
          </span>
          <Button size={28} onClick={() => props.onLoad()}>
            Load diff
          </Button>
        </div>
      </Show>
      <Show when={props.open && (!large() || props.loaded)}>
        <Hunks file={props.file} />
      </Show>
    </div>
  );
}
