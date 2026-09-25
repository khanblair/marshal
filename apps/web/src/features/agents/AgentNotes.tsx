import { For, Show } from "solid-js";
import { M } from "~/mock";
import { agentNotes } from "./agent-picker";

export interface AgentNotesProps {
  /** The agent the picker shows now, so an untested version's warning shows while it is chosen. */
  selected?: string;
  /** The id the picker names in `aria-describedby`. */
  id: string;
}

/**
 * Small helper text under an agent picker: the install command of each agent that is not installed
 * (its row is disabled) and the warning of the chosen agent when its version is untested. The
 * sentences come from the daemon's catalog. Draws nothing when there is nothing to say.
 */
export function AgentNotes(props: AgentNotesProps) {
  const notes = () => agentNotes(M.agentOptions(), props.selected);
  return (
    <Show when={notes().length > 0}>
      <div id={props.id} class="flex flex-col gap-0.5 text-small leading-4.5 text-secondary">
        <For each={notes()}>{(note) => <span>{note}</span>}</For>
      </div>
    </Show>
  );
}
