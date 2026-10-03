import { Button, Dialog, SegmentedControl, type SegmentOption } from "@marshal/ui";
import type { JSX } from "solid-js";
import { M } from "~/mock";

/**
 * The shell of a connection dialog that has two ways in, as a tab each (GitHub's own is the first of
 * these): a title, the tabs, the open tab's panel, and Close. The caller owns which tab is open, so
 * it can pick the first one from what it learns about the connection.
 */
export function WaysDialog<W extends string>(props: {
  /** The id of the title, which the dialog is labelled by. */
  titleId: string;
  title: string;
  /** What the tab strip is for, read out to a screen reader. */
  label: string;
  ways: readonly SegmentOption<W>[];
  value: W;
  onValueChange: (way: W) => void;
  onClose: () => void;
  children: JSX.Element;
}) {
  return (
    <Dialog width={560} phone={M.mobile} aria-labelledby={props.titleId} onClose={props.onClose}>
      <h2 id={props.titleId} class="m-0 text-title leading-6 font-semibold">
        {props.title}
      </h2>
      <SegmentedControl
        kind="tabs"
        size={30}
        fill
        unselectedTone="primary"
        label={props.label}
        options={props.ways}
        value={props.value}
        onValueChange={props.onValueChange}
      />
      <div role="tabpanel" aria-label={props.ways.find((way) => way.value === props.value)?.label}>
        {props.children}
      </div>
      <div class="flex justify-end gap-2">
        <Button onClick={props.onClose}>Close</Button>
      </div>
    </Dialog>
  );
}
