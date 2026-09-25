import { Avatar, Icon, Menu, MenuItem } from "@marshal/ui";
import { For, Index, Show } from "solid-js";
import { type Card, M } from "~/mock";
import type { Panel } from "./panel-state";

export interface MembersRowProps {
  card: Card;
  panel: Panel;
}

const initialsOf = (name: string): string =>
  name
    .split(" ")
    .map((part) => part[0])
    .join("")
    .slice(0, 2);
const firstName = (name: string): string => name.split(" ")[0] ?? name;

const CHIP = "inline-flex items-center gap-1.5 h-7 pr-2 pl-0.5 border border-border text-small";

/** The people on the card, then the agent (once the card has started). */
function MemberChips(props: { card: Card }) {
  const people = () =>
    props.card.members.flatMap((id) => {
      const person = M.person(id);
      return person ? [person.name] : [];
    });
  return (
    <>
      <For each={people()}>
        {(name) => (
          <span title={name} class={`${CHIP} rounded-full`}>
            <Avatar initials={initialsOf(name)} />
            {firstName(name)}
          </span>
        )}
      </For>
      <Show when={props.card.state !== "backlog"}>
        <span
          title={`${props.card.agent} agent, ${props.card.role} role`}
          class={`${CHIP} rounded-md`}
        >
          <Avatar kind="agent" class="rounded-sm!" />
          {props.card.agent}
        </span>
      </Show>
    </>
  );
}

const MENU_HEADING = "px-2 text-caption font-semibold text-secondary";

/** The menu that adds and removes people. The agent is listed with a note. */
function MembersMenu(props: MembersRowProps) {
  return (
    <Menu class="absolute left-0 top-[calc(100%+4px)] w-65 z-menu">
      <div class={`pt-1.5 pb-1 ${MENU_HEADING}`}>People</div>
      <Index each={M.S.people}>
        {(person) => {
          const on = () => props.card.members.includes(person().id);
          return (
            <MenuItem
              kind="check"
              size={34}
              checked={on()}
              leading={<Avatar initials={initialsOf(person().name)} />}
              onClick={() => M.toggleMember(props.card.id, person().id)}
            >
              {person().name}
            </MenuItem>
          );
        }}
      </Index>
      <div class={`pt-2 pb-1 ${MENU_HEADING}`}>Agent</div>
      <div class="flex items-center gap-2 pt-1 pr-2 pb-1.5 pl-2 text-small text-secondary">
        <Icon name="bot" size={14} />
        {props.card.state === "backlog"
          ? "The agent joins when the card starts"
          : `${props.card.agent} is on this card. Change it in Agent below.`}
      </div>
    </Menu>
  );
}

/** "Members" with chips, a dashed add button, and the member picker. */
export function MembersRow(props: MembersRowProps) {
  return (
    <div class="flex flex-wrap items-center gap-1.5 relative">
      <span class="text-small text-secondary mr-1">Members</span>
      <MemberChips card={props.card} />
      <button
        type="button"
        onClick={() => props.panel.set({ membersOpen: !props.panel.state.membersOpen })}
        aria-label="Add or remove members"
        aria-expanded={props.panel.state.membersOpen}
        title="Add or remove members"
        class="size-7 inline-flex items-center justify-center rounded-full border border-dashed border-border-strong bg-transparent text-secondary hover:bg-surface-hover"
      >
        <Icon name="plus" size={14} />
      </button>
      <Show when={props.panel.state.membersOpen}>
        <MembersMenu card={props.card} panel={props.panel} />
      </Show>
    </div>
  );
}
