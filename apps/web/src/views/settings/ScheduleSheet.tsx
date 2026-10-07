import type { ScheduleCatalog, ScheduleRun } from "@marshal/protocol";
import { Button, Dialog, Icon, IconButton } from "@marshal/ui";
import { createResource, createSignal, createUniqueId, For, Match, Show, Switch } from "solid-js";
import { Markdown } from "~/features/markdown/Markdown";
import { M, type Schedule } from "~/mock";
import type { EditState } from "./edit-state";
import { ScheduleEditor } from "./ScheduleEditor";

export type SheetView = "edit" | "history" | "preview";

const VIEW_WORDS: Record<SheetView, string> = {
  edit: "Edit",
  history: "History",
  preview: "Message preview",
};

/** How many runs the history lists. */
const RECENT_RUNS = 10;

/** A run's details are markdown, with headings sized to fit a small card rather than a page. */
const DETAILS_CLASS =
  "text-secondary [&_h1]:mb-1 [&_h1]:text-lead [&_h1]:text-primary [&_h2]:mt-3 [&_h2]:mb-1 [&_h2]:text-small [&_h2]:text-primary";

const runTone = (status: string): string => {
  if (status === "failed") return "text-status-danger-text";
  return status === "unsupported" ? "text-status-needs-you-text" : "text-secondary";
};

/** The first line of a run's saved text, without its markdown marks: what a closed run shows. */
const firstLine = (details: string): string =>
  details
    .split("\n")
    .map((line) => line.replace(/^#+\s*/, "").trim())
    .find((line) => line !== "") ?? "";

/** One run. Its saved message folds away under its time and status, and starts closed. */
function Run(props: { run: ScheduleRun }) {
  const [open, setOpen] = createSignal(false);
  const bodyId = createUniqueId();
  const title = () => (
    <span class={`shrink-0 ${runTone(props.run.status)}`}>
      {new Date(props.run.runAt).toLocaleString()} - {props.run.status}
    </span>
  );
  return (
    <div class="flex flex-col rounded-md border border-border bg-surface-sunken text-small">
      <Show
        when={props.run.details}
        fallback={<div class="flex min-h-8 items-center p-3">{title()}</div>}
      >
        {(details) => (
          <>
            <button
              type="button"
              aria-expanded={open()}
              aria-controls={bodyId}
              onClick={() => setOpen(!open())}
              class="flex min-h-8 w-full items-center gap-2 rounded-md border-none bg-transparent p-3 text-left hover:bg-surface-hover"
            >
              <span class="inline-flex text-muted">
                <Icon name={open() ? "chevron-down" : "chevron-right"} size={14} />
              </span>
              {title()}
              <Show when={!open()}>
                <span class="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap text-muted">
                  {firstLine(details())}
                </span>
              </Show>
            </button>
            <Show when={open()}>
              <div id={bodyId} class="px-3 pb-3">
                <Markdown text={details()} class={DETAILS_CLASS} />
              </div>
            </Show>
          </>
        )}
      </Show>
    </div>
  );
}

function History(props: { id: string }) {
  const [runs] = createResource(() => M.scheduleRuns(props.id));
  return (
    <div class="flex flex-col gap-2">
      <Show when={runs()} fallback={<span class="text-secondary">Reading the history...</span>}>
        {(list) => (
          <Show
            when={list().length > 0}
            fallback={<span class="text-secondary">Never run yet.</span>}
          >
            <For each={list().slice(0, RECENT_RUNS)}>{(run) => <Run run={run} />}</For>
          </Show>
        )}
      </Show>
    </div>
  );
}

function Preview(props: { id: string }) {
  const [text] = createResource(() => M.previewSchedule(props.id));
  return (
    <div class="flex flex-col gap-2">
      <span class="text-small leading-4.5 text-secondary">
        What Telegram, Discord, and ntfy would show if it were sent now, from the saved schedule.
        Nothing is sent.
      </span>
      <Show when={!text.loading} fallback={<span class="text-secondary">Writing it...</span>}>
        <Show
          when={text()}
          fallback={<span class="text-secondary">This schedule cannot be previewed.</span>}
        >
          {(message) => (
            <pre class="m-0 whitespace-pre-wrap break-words rounded-md border border-border bg-surface-sunken p-3 font-sans text-body leading-5">
              {message()}
            </pre>
          )}
        </Show>
      </Show>
    </div>
  );
}

interface ScheduleSheetProps {
  schedule: Schedule;
  view: SheetView;
  edit: EditState;
  catalog: ScheduleCatalog | null;
  onClose: () => void;
}

/**
 * The sheet a schedule's editor, history, and message preview open in: a bottom sheet on a phone and a
 * centered panel elsewhere. The editor's Save is in the header too, so it is in reach however long the
 * form is.
 */
export function ScheduleSheet(props: ScheduleSheetProps) {
  const titleId = createUniqueId();
  const formId = createUniqueId();
  return (
    <Dialog
      phone={M.mobile}
      width={560}
      aria-labelledby={titleId}
      onClose={props.onClose}
      class="gap-4!"
    >
      <div class="flex items-center gap-2">
        <div class="flex min-w-0 flex-1 flex-col">
          <h2 id={titleId} class="m-0 truncate text-title leading-6 font-semibold">
            {props.schedule.name}
          </h2>
          <span class="text-small leading-4.5 text-secondary">{VIEW_WORDS[props.view]}</span>
        </div>
        <Show when={props.view === "edit"}>
          <Button variant="primary" size={28} type="submit" form={formId}>
            Save
          </Button>
        </Show>
        <IconButton label="Close" icon="x" onClick={props.onClose} />
      </div>
      <Switch>
        <Match when={props.view === "edit"}>
          <ScheduleEditor
            formId={formId}
            schedule={props.schedule}
            edit={props.edit}
            catalog={props.catalog}
          />
        </Match>
        <Match when={props.view === "history"}>
          <History id={props.schedule.id} />
        </Match>
        <Match when={props.view === "preview"}>
          <Preview id={props.schedule.id} />
        </Match>
      </Switch>
    </Dialog>
  );
}
