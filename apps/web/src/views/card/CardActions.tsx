import { Icon, IconButton, Kbd, Menu, MenuItem } from "@marshal/ui";
import { For, Index, Show } from "solid-js";
import { type Card, M } from "~/mock";
import { GoogleImportDialog } from "~/views/google-export/GoogleImportDialog";
import { type CardAction, cardActions, moreItems } from "./card-actions";
import { controlSize } from "./control-size";
import type { Panel } from "./panel-state";

export interface CardActionsProps {
  card: Card;
  panel: Panel;
  /** Only the action that leads (Approve, Resume agent, Start), for a card whose details are folded away. */
  primaryOnly?: boolean;
}

const BASE =
  "inline-flex items-center gap-1.5 px-2.5 rounded-sm border text-small hover:brightness-97";
const PRIMARY = "border-ink bg-ink text-on-ink font-semibold";
const SECONDARY = "border-border-strong bg-surface text-primary font-medium";

function ActionButton(props: { action: CardAction }) {
  return (
    <button
      type="button"
      onClick={() => props.action.run()}
      disabled={props.action.disabled}
      title={props.action.label}
      class={`${BASE} h-7 phone:h-(--control-h) ${props.action.primary ? PRIMARY : SECONDARY}`}
    >
      <Icon name={props.action.icon} size={14} />
      {props.action.label}
      <Show when={props.action.kbd}>
        <Kbd tone="current" size="sm">
          {props.action.kbd}
        </Kbd>
      </Show>
    </button>
  );
}

function MoreMenu(props: CardActionsProps) {
  const items = () =>
    moreItems(
      props.card,
      () => props.panel.set({ more: false }),
      () => props.panel.set({ importOpen: true }),
    );
  return (
    <Menu class="absolute top-[calc(100%+4px)] right-0 w-60 max-h-90 overflow-auto z-menu">
      <Index each={items()}>
        {(item) => (
          <MenuItem
            icon={item().icon}
            class={item().danger ? "text-status-danger-text!" : undefined}
            disabled={item().disabled}
            hint={item().hint}
            onClick={() => item().run()}
          >
            {item().label}
          </MenuItem>
        )}
      </Index>
    </Menu>
  );
}

/** Approve, start or pause, sleep, pin, fork, merge, the More actions menu, and its Google import dialog. */
export function CardActions(props: CardActionsProps) {
  const actions = () =>
    cardActions(props.card, M.mobile).filter((action) => !props.primaryOnly || action.primary);
  return (
    <Show when={actions().length > 0 || !props.primaryOnly}>
      <div class="flex flex-wrap gap-1.5 relative">
        <For each={actions()}>{(action) => <ActionButton action={action} />}</For>
        <Show when={!props.primaryOnly}>
          <MoreActions card={props.card} panel={props.panel} />
        </Show>
      </div>
    </Show>
  );
}

/** The More actions button, its menu, and the Google import dialog the menu opens. */
function MoreActions(props: CardActionsProps) {
  return (
    <>
      <div class="relative">
        <IconButton
          label="More actions"
          icon="ellipsis"
          variant="outline"
          size={controlSize()}
          aria-expanded={props.panel.state.more}
          onClick={() => props.panel.set({ more: !props.panel.state.more })}
        />
        <Show when={props.panel.state.more}>
          <MoreMenu card={props.card} panel={props.panel} />
        </Show>
      </div>
      <GoogleImportDialog card={props.card} panel={props.panel} />
    </>
  );
}
