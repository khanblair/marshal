import { Icon } from "@marshal/ui";
import { For, onMount, Show } from "solid-js";
import type { TrelloApplyReport, TrelloReportRow } from "~/data/mappers/trello";

const ICON_PX = 14;

/** One change in the report: what it did, on which card, and why it was left when it was. */
function ReportRow(props: { row: TrelloReportRow; reason?: string }) {
  return (
    <li class="flex flex-col gap-0.5 py-2">
      <span class="font-medium">{props.row.summary}</span>
      <Show when={props.row.cardTitle}>
        <span class="text-small text-secondary">{props.row.cardTitle}</span>
      </Show>
      <Show when={props.reason}>
        <span class="text-small text-status-needs-you-text">{props.reason}</span>
      </Show>
    </li>
  );
}

/** A heading with a count and the rows under it, for the changes that were written or left. */
function ReportList(props: {
  title: string;
  icon: "circle-check" | "triangle-alert";
  rows: readonly (TrelloReportRow & { reason?: string })[];
}) {
  return (
    <Show when={props.rows.length > 0}>
      <section aria-label={props.title} class="flex flex-col">
        <h3 class="m-0 flex items-center gap-1.5 text-body-strong">
          <Icon name={props.icon} size={ICON_PX} />
          {props.title} ({props.rows.length})
        </h3>
        <ul class="m-0 flex list-none flex-col divide-y divide-border p-0">
          <For each={props.rows}>{(row) => <ReportRow row={row} reason={row.reason} />}</For>
        </ul>
      </section>
    </Show>
  );
}

/**
 * What an Apply did: the changes that were written to Trello, and the ones that were not, each with
 * the daemon's reason. It takes focus when it appears, so a screen reader reads it and the keyboard
 * stays inside the dialog.
 */
export function TrelloResult(props: { report: TrelloApplyReport; headingId: string }) {
  let heading: HTMLHeadingElement | undefined;
  onMount(() => heading?.focus());
  const none = () => props.report.applied.length === 0 && props.report.skipped.length === 0;
  return (
    <div role="status" class="flex min-h-0 flex-col gap-4">
      <h3 ref={heading} id={props.headingId} tabindex="-1" class="m-0 text-subtitle">
        {props.report.applied.length === 0 ? "Nothing was written to Trello" : "Sent to Trello"}
      </h3>
      <Show when={none()}>
        <p class="m-0 text-secondary">No changes were picked.</p>
      </Show>
      <ReportList title="Written to Trello" icon="circle-check" rows={props.report.applied} />
      <ReportList title="Not written" icon="triangle-alert" rows={props.report.skipped} />
    </div>
  );
}
