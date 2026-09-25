import { createEffect, createMemo, Match, Show, Switch } from "solid-js";
import { type Card, M } from "~/mock";
import { ActivityTab } from "./ActivityTab";
import { BypassBanner } from "./BypassBanner";
import { CardHeader } from "./CardHeader";
import { ChatTab } from "./ChatTab";
import { ChecklistsTab } from "./ChecklistsTab";
import { CommentsTab } from "./CommentsTab";
import { ensureNote } from "./card-note";
import { DiffTab } from "./DiffTab";
import { NotesTab } from "./NotesTab";
import { PreviewTab } from "./PreviewTab";
import { createPanelState } from "./panel-state";
import type { TerminalState } from "./terminal-state";

export interface CardPanelProps {
  card: Card;
  terminal: TerminalState;
}

/** The open card: bypass banner, header, and the tab that is selected. */
export function CardPanel(props: CardPanelProps) {
  const panel = createPanelState();
  const c = createMemo(() => M.deco(props.card));
  createEffect(() => ensureNote(props.card));
  return (
    <>
      <Show when={props.card.bypass}>
        <BypassBanner cardId={props.card.id} />
      </Show>
      <CardHeader card={props.card} c={c()} panel={panel} />
      <div
        role="tabpanel"
        aria-labelledby={`tab-${M.S.tab}`}
        class="flex-1 min-h-0 flex flex-col relative"
      >
        <Switch>
          <Match when={M.S.tab === "chat"}>
            <ChatTab card={props.card} c={c()} panel={panel} terminal={props.terminal} />
          </Match>
          <Match when={M.S.tab === "comments"}>
            <CommentsTab card={props.card} panel={panel} />
          </Match>
          <Match when={M.S.tab === "activity"}>
            <ActivityTab card={props.card} c={c()} />
          </Match>
          <Match when={M.S.tab === "diff"}>
            <DiffTab card={props.card} panel={panel} />
          </Match>
          <Match when={M.S.tab === "checks"}>
            <ChecklistsTab card={props.card} panel={panel} />
          </Match>
          <Match when={M.S.tab === "preview"}>
            <PreviewTab card={props.card} />
          </Match>
          <Match when={M.S.tab === "notes"}>
            <NotesTab card={props.card} panel={panel} />
          </Match>
        </Switch>
      </div>
    </>
  );
}
