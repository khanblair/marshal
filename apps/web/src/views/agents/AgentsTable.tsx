import { SortHeader, sortDirection, TableRow, TableShell, toggleSort } from "@marshal/ui";
import { createMemo, For } from "solid-js";
import { type Card, M } from "~/mock";
import { AgentCells } from "./AgentCells";
import { agentRowOf, orchestratorRow } from "./agent-row";
import { columnsFor, NARROW_TABLE_MIN_WIDTH_PX, WIDE_TABLE_MIN_WIDTH_PX } from "./agents-model";

export interface AgentsTableProps {
  cards: readonly Card[];
  projectName: string;
  /** Show the Thinking, Permission mode, and Session columns. */
  wide: boolean;
}

function AgentsHeader(props: { wide: boolean }) {
  return (
    <For each={columnsFor(props.wide)}>
      {(column) => (
        <SortHeader
          label={column.label}
          class={column.class}
          sort={sortDirection(M.S.sort.agents, column.key)}
          onSort={
            column.key
              ? () => {
                  M.S.sort.agents = toggleSort(M.S.sort.agents, column.key);
                }
              : undefined
          }
        />
      )}
    </For>
  );
}

function OrchestratorRow(props: { projectName: string; wide: boolean }) {
  const row = createMemo(() => orchestratorRow(props.projectName));
  return (
    <TableRow
      data-card=""
      aria-label={row().aria}
      onClick={() => M.setView("chat")}
      onKeyDown={(e) => {
        if (e.key === "Enter") M.setView("chat");
      }}
    >
      <AgentCells row={row()} wide={props.wide} />
    </TableRow>
  );
}

function AgentCardRow(props: { card: Card; wide: boolean }) {
  const row = createMemo(() => agentRowOf(props.card));
  const open = () => M.openCard(props.card.id);
  return (
    <TableRow
      data-card={props.card.id}
      aria-label={row().aria}
      highlight={M.S.focusId === props.card.id}
      onClick={open}
      onKeyDown={(e) => {
        if (e.key === "Enter" && e.target === e.currentTarget) open();
      }}
    >
      <AgentCells row={row()} wide={props.wide} />
    </TableRow>
  );
}

/** The sessions table: the Orchestrator first, then one row per card that has a session. */
export function AgentsTable(props: AgentsTableProps) {
  return (
    <TableShell
      minWidth={props.wide ? WIDE_TABLE_MIN_WIDTH_PX : NARROW_TABLE_MIN_WIDTH_PX}
      header={<AgentsHeader wide={props.wide} />}
    >
      <OrchestratorRow projectName={props.projectName} wide={props.wide} />
      <For each={props.cards}>{(card) => <AgentCardRow card={card} wide={props.wide} />}</For>
    </TableShell>
  );
}
