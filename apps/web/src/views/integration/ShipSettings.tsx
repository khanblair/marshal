import type { IntegrationSettings } from "@marshal/protocol";
import { Switch } from "@marshal/ui";
import { createSignal, createUniqueId, For } from "solid-js";
import { changeShipSettings } from "~/sync/integration-flow";

/** One switch of how a project ships its cards: the name, what it does, and which setting it is. */
interface Row {
  key: keyof IntegrationSettings;
  label: string;
  hint: string;
}

/** The switches in the order a card meets them. Everything but pushing is on until it is turned off. */
export const SHIP_ROWS: readonly Row[] = [
  {
    key: "autoShip",
    label: "Ship finished cards by themselves",
    hint: "When an agent says it is done, review, checks and the merge follow without you.",
  },
  {
    key: "review",
    label: "Review changes",
    hint: "A model reads each card's changes before they can merge, and sends them back if they need work.",
  },
  {
    key: "checks",
    label: "Run the project's checks",
    hint: "The project's own workflow files run on each card's changes, on this machine.",
  },
  {
    key: "autoMerge",
    label: "Merge ready cards",
    hint: "A card that is ready lands in your branch and folder by itself.",
  },
  {
    key: "allowPush",
    label: "Allow pushing to GitHub",
    hint: "Off, nothing leaves this machine. On, agents may push and Marshal may open pull requests.",
  },
];

/** One row: its words, and a switch that asks the daemon and shows what it answers. */
function ShipRow(props: { projectId: string; row: Row; on: boolean }) {
  const [busy, setBusy] = createSignal(false);
  const id = createUniqueId();
  async function change(next: boolean): Promise<void> {
    setBusy(true);
    try {
      await changeShipSettings(props.projectId, { [props.row.key]: next });
    } finally {
      setBusy(false);
    }
  }
  return (
    <label for={id} class="flex items-center gap-3 py-2.5">
      <span class="flex min-w-0 flex-1 flex-col">
        <span class="font-medium">{props.row.label}</span>
        <span class="text-small leading-4.5 text-secondary">{props.row.hint}</span>
      </span>
      <Switch
        id={id}
        checked={props.on}
        disabled={busy()}
        label={props.row.label}
        onCheckedChange={(next) => void change(next)}
      />
    </label>
  );
}

/** How the project ships its cards. Every change is the daemon's, and the answer is what is shown. */
export function ShipSettings(props: { projectId: string; settings: IntegrationSettings }) {
  return (
    <section
      aria-label="How cards ship"
      class="flex flex-col rounded-md border border-border bg-surface px-4 py-2"
    >
      <h3 class="m-0 py-1 text-small font-semibold text-secondary">How cards ship</h3>
      <div class="flex flex-col divide-y divide-border">
        <For each={SHIP_ROWS}>
          {(row) => <ShipRow projectId={props.projectId} row={row} on={props.settings[row.key]} />}
        </For>
      </div>
    </section>
  );
}
