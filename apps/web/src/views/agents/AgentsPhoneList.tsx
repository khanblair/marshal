import {
  cx,
  IconLabel,
  PhoneList,
  PhoneRowMeta,
  PhoneRowTitle,
  StatusDot,
  StatusLabel,
} from "@marshal/ui";
import { createMemo, For, Show } from "solid-js";
import { type Card, M } from "~/mock";
import { type AgentRowData, agentRowOf, orchestratorRow } from "./agent-row";
import { PhoneActionButtons } from "./PhoneActionButtons";

export interface AgentsPhoneListProps {
  cards: readonly Card[];
  projectName: string;
}

/** One session on a phone: the open button with the facts, then the action buttons. */
function AgentPhoneRow(props: { row: AgentRowData; onOpen: () => void }) {
  return (
    <li class="flex flex-col gap-1.5 py-3 border-b border-border">
      <button
        type="button"
        onClick={() => props.onOpen()}
        class="flex flex-col gap-1 border-none bg-transparent p-0 text-left"
      >
        <PhoneRowTitle title={props.row.title} num={props.row.num} />
        <PhoneRowMeta class="text-secondary">
          <StatusLabel state={props.row.state}>{props.row.stateLabel}</StatusLabel>
          <IconLabel icon={props.row.sessIcon}>{props.row.sess}</IconLabel>
          <span>{props.row.role}</span>
          <span>{props.row.agent}</span>
          <span>{props.row.model}</span>
          <span>{props.row.cost}</span>
        </PhoneRowMeta>
        <PhoneRowMeta class={props.row.bypass ? "text-status-danger-text" : undefined}>
          <span>{props.row.perm}</span>
          <span class="text-secondary">{props.row.think}</span>
        </PhoneRowMeta>
        <span
          class={cx("flex items-center gap-1.5 text-small leading-4.5", props.row.activityClass)}
        >
          <Show when={props.row.pulse}>
            <StatusDot state="working" />
          </Show>
          {props.row.activity}
        </span>
      </button>
      <PhoneActionButtons actions={props.row.actions} />
    </li>
  );
}

function OrchestratorPhoneRow(props: { projectName: string }) {
  const row = createMemo(() => orchestratorRow(props.projectName));
  return <AgentPhoneRow row={row()} onOpen={() => M.setView("chat")} />;
}

function CardPhoneRow(props: { card: Card }) {
  const row = createMemo(() => agentRowOf(props.card));
  return <AgentPhoneRow row={row()} onOpen={() => M.openCard(props.card.id)} />;
}

/** The sessions on a phone: a list of stacked facts with 44 px action buttons. */
export function AgentsPhoneList(props: AgentsPhoneListProps) {
  return (
    <PhoneList>
      <OrchestratorPhoneRow projectName={props.projectName} />
      <For each={props.cards}>{(card) => <CardPhoneRow card={card} />}</For>
    </PhoneList>
  );
}
