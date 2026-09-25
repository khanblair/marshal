import { SortHeader, sortDirection, TableRow, TableShell, toggleSort } from "@marshal/ui";
import { createMemo, For } from "solid-js";
import { Dynamic } from "solid-js/web";
import { type Card, M } from "~/mock";
import { LIST_CELLS } from "./ListCells";
import { type ListColumn, MIN_COLUMN_WIDTH_PX } from "./list-columns";

export interface ListTableProps {
  cards: readonly Card[];
  columns: readonly ListColumn[];
}

function ListHeader(props: { columns: readonly ListColumn[] }) {
  return (
    <For each={props.columns}>
      {(column) => (
        <SortHeader
          size={34}
          label={column.label}
          alignEnd={column.alignEnd}
          sort={sortDirection(M.S.sort.list, column.key)}
          onSort={() => {
            M.S.sort.list = toggleSort(M.S.sort.list, column.key);
          }}
        />
      )}
    </For>
  );
}

function ListRow(props: { card: Card; columns: readonly ListColumn[] }) {
  const deco = createMemo(() => M.deco(props.card));
  const open = () => M.openCard(props.card.id);
  return (
    <TableRow
      data-card={props.card.id}
      aria-label={deco().aria}
      highlight={M.S.focusId === props.card.id}
      onClick={open}
      onKeyDown={(e) => {
        if (e.key === "Enter") open();
      }}
    >
      <For each={props.columns}>
        {(column) => <Dynamic component={LIST_CELLS[column.key]} card={props.card} deco={deco()} />}
      </For>
    </TableRow>
  );
}

/** The cards as a table with the columns turned on in the Columns menu. */
export function ListTable(props: ListTableProps) {
  return (
    <TableShell
      minWidth={props.columns.length * MIN_COLUMN_WIDTH_PX}
      header={<ListHeader columns={props.columns} />}
    >
      <For each={props.cards}>{(card) => <ListRow card={card} columns={props.columns} />}</For>
    </TableShell>
  );
}
