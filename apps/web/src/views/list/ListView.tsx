import { NoResults } from "@marshal/ui";
import { createMemo, createRenderEffect, onCleanup, Show } from "solid-js";
import { M } from "~/mock";
import { ListPhoneList } from "./ListPhoneList";
import { ListTable } from "./ListTable";
import { noMatchText, sortListCards, visibleColumns } from "./list-columns";

/**
 * The project's cards as a sortable table, with the columns chosen in the Columns menu. Phones
 * get a stacked list instead. When the filters hide every card a notice says so above the
 * headers.
 */
export function ListView() {
  const pid = () => M.S.route.pid ?? "";
  const cards = createMemo(() => sortListCards(M.filtered(pid()), M.S.sort.list));
  const columns = createMemo(() => visibleColumns(M.S.listCols, M.S.vw));
  createRenderEffect(() => {
    M.nav = { owner: "list", rows: cards().map((card) => card.id) };
  });
  onCleanup(() => {
    if (M.nav?.owner === "list") M.nav = null;
  });
  return (
    <div class="absolute inset-0 overflow-auto bg-surface">
      <Show when={cards().length === 0}>
        <NoResults class="m-4" onClear={() => M.clearFilters()}>
          {noMatchText(M.S.query[pid()])}
        </NoResults>
      </Show>
      <Show when={M.mobile} fallback={<ListTable cards={cards()} columns={columns()} />}>
        <ListPhoneList cards={cards()} />
      </Show>
    </div>
  );
}
