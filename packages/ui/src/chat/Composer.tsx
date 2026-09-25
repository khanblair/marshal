import { type JSX, splitProps } from "solid-js";
import { cx } from "../base/cx";
import { IconButton } from "../base/IconButton";

export interface ComposerProps extends Omit<JSX.HTMLAttributes<HTMLDivElement>, "onInput"> {
  /** The draft text. */
  value: string;
  onValueChange: (value: string) => void;
  /** Called on Enter (without Shift) and on the send button, when the draft is not blank. */
  onSend: () => void;
  placeholder?: string;
  /** Accessible name of the text field, such as `Message the agent`. */
  label: string;
  /** Send button size in px: 28 (card panel), 32 (default, chats), or 44 (phone). */
  sendSize?: 28 | 32 | 44;
  /** Tallest the field grows before it scrolls, in px: 140 or 160 (default). */
  maxHeight?: 140 | 160;
  /** Gets the textarea, to focus it. */
  textareaRef?: (el: HTMLTextAreaElement) => void;
}

const SHORT_MAX = 140;
const DEFAULT_SEND_PX = 32;

/**
 * The chat message box: a borderless growing text field and an ink send
 * button inside one rounded border. Enter sends; Shift and Enter adds a line.
 */
export function Composer(props: ComposerProps) {
  const [local, others] = splitProps(props, [
    "value",
    "onValueChange",
    "onSend",
    "placeholder",
    "label",
    "sendSize",
    "maxHeight",
    "textareaRef",
    "class",
  ]);
  const blank = () => !local.value.trim();
  const send = () => {
    if (!blank()) local.onSend();
  };
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key !== "Enter" || event.shiftKey) return;
    event.preventDefault();
    send();
  };
  return (
    <div
      {...others}
      class={cx(
        "flex items-end gap-2 py-1.5 pr-1.5 pl-3 rounded-lg border border-border-strong bg-surface",
        local.class,
      )}
    >
      <textarea
        ref={(el) => local.textareaRef?.(el)}
        rows={1}
        value={local.value}
        onInput={(event) => local.onValueChange(event.currentTarget.value)}
        onKeyDown={onKeyDown}
        placeholder={local.placeholder}
        aria-label={local.label}
        class={cx(
          "flex-1 min-h-7 py-1 px-0 border-none outline-none bg-transparent resize-none",
          local.maxHeight === SHORT_MAX ? "max-h-35" : "max-h-40",
        )}
      />
      <IconButton
        label="Send message"
        icon="send-horizontal"
        variant="primary"
        size={local.sendSize ?? DEFAULT_SEND_PX}
        disabled={blank()}
        onClick={send}
      />
    </div>
  );
}
