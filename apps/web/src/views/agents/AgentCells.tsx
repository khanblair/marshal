import { Badge, cx, IconLabel, StatusDot, StatusLabel, TableCell } from "@marshal/ui";
import { Show } from "solid-js";
import type { AgentRowData } from "./agent-row";
import { TableActionButtons } from "./TableActionButtons";

export interface AgentCellsProps {
  row: AgentRowData;
  /** Show the Thinking, Permission mode, and Session columns. */
  wide: boolean;
}

const TEXT_CELL = "px-3 py-2 whitespace-nowrap";

function CardCell(props: { row: AgentRowData }) {
  return (
    <TableCell class="px-3 py-2 max-w-[320px]">
      <div class="flex gap-2 items-baseline min-w-0">
        <span class="flex-none text-muted text-caption">{props.row.num}</span>
        <span class="text-body font-semibold truncate">{props.row.title}</span>
      </div>
      <Show when={props.row.bypass}>
        <Badge tone="bypass" size={18} icon="shield-alert" class="mt-1">
          Bypass
        </Badge>
      </Show>
    </TableCell>
  );
}

function SessionCell(props: { row: AgentRowData }) {
  return (
    <TableCell class={TEXT_CELL}>
      <IconLabel icon={props.row.sessIcon} class="text-secondary">
        {props.row.sess}
      </IconLabel>
      <Show when={props.row.pinned}>
        <IconLabel icon="pin" size={12} gap={3} class="ml-2 text-secondary">
          Pinned
        </IconLabel>
      </Show>
    </TableCell>
  );
}

function ActivityCell(props: { row: AgentRowData }) {
  return (
    <TableCell class="px-3 py-2 max-w-[280px]">
      <div class="flex items-center gap-1.5 min-w-0">
        <Show when={props.row.pulse}>
          <StatusDot state="working" />
        </Show>
        <span class={cx("truncate", props.row.activityClass)}>{props.row.activity}</span>
      </div>
    </TableCell>
  );
}

/** The cells of one session row, from Card to Actions. */
export function AgentCells(props: AgentCellsProps) {
  return (
    <>
      <CardCell row={props.row} />
      <TableCell class={TEXT_CELL}>{props.row.role}</TableCell>
      <TableCell class={TEXT_CELL}>{props.row.agent}</TableCell>
      <TableCell class={TEXT_CELL}>{props.row.model}</TableCell>
      <Show when={props.wide}>
        <TableCell class={cx(TEXT_CELL, "text-secondary")}>{props.row.think}</TableCell>
        <TableCell
          class={cx(TEXT_CELL, props.row.bypass && "text-status-danger-text font-semibold")}
        >
          {props.row.perm}
        </TableCell>
      </Show>
      <TableCell class={TEXT_CELL}>
        <StatusLabel state={props.row.state}>{props.row.stateLabel}</StatusLabel>
      </TableCell>
      <Show when={props.wide}>
        <SessionCell row={props.row} />
      </Show>
      <ActivityCell row={props.row} />
      <TableCell class={cx(TEXT_CELL, "text-right tabular-nums")}>{props.row.cost}</TableCell>
      <TableCell class="px-2 py-1 whitespace-nowrap">
        <TableActionButtons actions={props.row.actions} />
      </TableCell>
    </>
  );
}
