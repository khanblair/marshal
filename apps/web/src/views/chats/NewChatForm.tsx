import { Button, Select } from "@marshal/ui";
import { M } from "~/mock";
import { newChatTargets, ORCHESTRATOR } from "./chat-model";

export interface NewChatFormProps {
  /** Called after the chat is created, to focus the message box. */
  onStarted: () => void;
}

const DESKTOP_POPOVER =
  "absolute top-9.5 right-0 left-0 z-[60] flex flex-col gap-3 rounded-lg border border-border bg-surface-raised p-3.5 shadow-e1";
const PHONE_SHEET =
  "fixed right-0 bottom-0 left-0 z-sheet flex flex-col gap-3 rounded-t-xl bg-surface-raised px-4 pt-5 pb-[calc(20px+env(safe-area-inset-bottom))] shadow-e2";

const closeForm = () => M.set({ newChatOpen: false });

/** Who to talk to. On a phone it is a bottom sheet, on larger screens a popover under the search. */
export function NewChatForm(props: NewChatFormProps) {
  const pid = () => M.S.route.pid ?? "";
  const start = (event: SubmitEvent & { currentTarget: HTMLFormElement }) => {
    event.preventDefault();
    // The new chat replaces the list on a phone and removes this form, so read everything first.
    const target = String(new FormData(event.currentTarget).get("target") ?? ORCHESTRATOR);
    const { onStarted } = props;
    M.newChat(pid(), target);
    closeForm();
    onStarted();
  };
  return (
    <form onSubmit={start} class={M.mobile ? PHONE_SHEET : DESKTOP_POPOVER}>
      <label class="flex flex-col gap-1.5">
        <span class="font-semibold">Who do you want to talk to?</span>
        <Select name="target" value={ORCHESTRATOR} options={newChatTargets(pid())} />
        <span class="text-small leading-4.5 text-secondary">
          The Orchestrator plans work and creates cards on this project's board.
        </span>
      </label>
      <div class="flex gap-2">
        <Button variant="primary" type="submit" class="hover:bg-ink!">
          Start chat
        </Button>
        <Button onClick={closeForm} class="hover:bg-surface!">
          Cancel
        </Button>
      </div>
    </form>
  );
}
