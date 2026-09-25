import { CiStatus, cx, IconLabel, StatusLabel, TableCell } from "@marshal/ui";
import { type Component, Show } from "solid-js";
import type { Card, CardView } from "~/mock";
import type { ListKey } from "./list-columns";

interface ListCellProps {
  card: Card;
  deco: CardView;
}

/** Every cell is one 36 px line that clips long text with an ellipsis. */
const CELL = "h-9 px-3 truncate";
const MONO_CELL = "font-mono text-caption";

function titleIcon(card: Card): { name: string; color: string } | undefined {
  if (card.bypass) return { name: "shield-alert", color: "text-status-danger-solid" };
  if (card.asleep) return { name: "moon", color: "text-secondary" };
  return card.pinned ? { name: "pin", color: "text-secondary" } : undefined;
}

function TitleCell(props: ListCellProps) {
  return (
    <TableCell
      class={cx(CELL, "max-w-[360px] font-semibold")}
      style={{ color: props.deco.titleColor }}
    >
      <Show when={titleIcon(props.card)} fallback={props.card.title}>
        {(icon) => (
          <IconLabel icon={icon().name} iconClass={icon().color}>
            {props.card.title}
          </IconLabel>
        )}
      </Show>
    </TableCell>
  );
}

function CiCell(props: ListCellProps) {
  return (
    <TableCell class={CELL}>
      <Show when={props.deco.hasCi}>
        <CiStatus status={props.card.ci ?? ""}>{props.deco.ciLabel}</CiStatus>
      </Show>
    </TableCell>
  );
}

/** The cell of each column. A missing package or branch leaves the cell empty. */
export const LIST_CELLS: Record<ListKey, Component<ListCellProps>> = {
  id: (props) => <TableCell class={cx(CELL, "text-muted")}>{props.deco.num}</TableCell>,
  title: TitleCell,
  state: (props) => (
    <TableCell class={CELL}>
      <StatusLabel state={props.card.state}>{props.deco.stateLabel}</StatusLabel>
    </TableCell>
  ),
  role: (props) => <TableCell class={CELL}>{props.card.role}</TableCell>,
  agent: (props) => <TableCell class={CELL}>{props.card.agent}</TableCell>,
  model: (props) => <TableCell class={CELL}>{props.card.model}</TableCell>,
  think: (props) => (
    <TableCell class={cx(CELL, "text-secondary")}>{props.deco.think || "Not supported"}</TableCell>
  ),
  pkg: (props) => <TableCell class={cx(CELL, MONO_CELL)}>{props.card.pkg || ""}</TableCell>,
  branch: (props) => (
    <TableCell class={cx(CELL, MONO_CELL, "text-secondary max-w-[260px]")}>
      {props.card.branch || ""}
    </TableCell>
  ),
  ci: CiCell,
  cost: (props) => (
    <TableCell class={cx(CELL, "text-right")}>
      {props.deco.hasCost ? props.deco.cost : ""}
    </TableCell>
  ),
  upd: (props) => <TableCell class={cx(CELL, "text-secondary")}>{props.deco.upd}</TableCell>,
};
