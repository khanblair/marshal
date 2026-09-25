import { Icon } from "@marshal/ui";
import { Index, Show } from "solid-js";
import { type Card, M } from "~/mock";
import { terminalLines } from "./terminal-lines";
import type { TerminalState } from "./terminal-state";

export interface TerminalViewProps {
  card: Card;
  terminal: TerminalState;
  scrollRef: (el: HTMLElement) => void;
}

interface TermKey {
  key: string;
  aria: string;
}

const TERM_KEYS: readonly TermKey[] = [
  { key: "Esc", aria: "Escape key" },
  { key: "Tab", aria: "Tab key" },
  { key: "Ctrl", aria: "Control key" },
  { key: "arrow-left", aria: "Left arrow" },
  { key: "arrow-up", aria: "Up arrow" },
  { key: "arrow-down", aria: "Down arrow" },
  { key: "arrow-right", aria: "Right arrow" },
];

const isArrow = (key: string): boolean => key.startsWith("arrow");

/** The phone's key bar: keys a touch keyboard lacks. Pressed keys are only logged. */
function KeyBar(props: { terminal: TerminalState; focusInput: () => void }) {
  return (
    <div
      role="toolbar"
      aria-label="Terminal keys"
      class="flex-none flex gap-1.5 overflow-x-auto pt-2 px-3 pb-[calc(8px+env(safe-area-inset-bottom))] border-t border-border bg-surface"
    >
      <Index each={TERM_KEYS}>
        {(item) => {
          const active = () => item().key === "Ctrl" && props.terminal.ctrl();
          return (
            <button
              type="button"
              aria-label={item().aria}
              onClick={() => {
                props.terminal.press(item().key, item().aria);
                if (item().key !== "Ctrl") props.focusInput();
              }}
              class={`flex-none min-w-13 h-11 px-3 inline-flex items-center justify-center rounded-sm border border-border-strong font-mono text-small font-semibold ${
                active() ? "bg-ink text-on-ink" : "bg-surface-sunken text-primary"
              }`}
            >
              <Show when={isArrow(item().key)} fallback={item().key}>
                <Icon name={item().key} size={16} />
              </Show>
            </button>
          );
        }}
      </Index>
    </div>
  );
}

/** The terminal mode: the session as plain text with a prompt. It is fake, as in the design. */
export function TerminalView(props: TerminalViewProps) {
  let input: HTMLInputElement | undefined;
  const focusInput = () => input?.focus();
  const lines = () =>
    terminalLines(props.card, M.S.chat[props.card.id] ?? [], props.terminal.keyLog());
  const submit = (e: SubmitEvent & { currentTarget: HTMLFormElement }) => {
    e.preventDefault();
    const field = e.currentTarget.elements.namedItem("cmd");
    if (!(field instanceof HTMLInputElement) || !field.value.trim()) return;
    M.send(props.card.id, field.value);
    field.value = "";
  };
  return (
    <>
      {/* biome-ignore lint/a11y/useKeyWithClickEvents: clicking the terminal only moves focus to its own input, which the keyboard already reaches */}
      {/* biome-ignore lint/a11y/noStaticElementInteractions: same as above */}
      <div
        ref={props.scrollRef}
        onClick={focusInput}
        class="flex-1 min-h-0 overflow-auto py-3 px-4 bg-surface-sunken font-mono text-small leading-5 whitespace-pre-wrap [word-break:break-word]"
      >
        <Index each={lines()}>{(line) => <div class={line().class}>{line().text}</div>}</Index>
        <form onSubmit={submit} class="flex gap-2 mt-1">
          <span class="text-status-working-text">&gt;</span>
          <input
            ref={input}
            name="cmd"
            enterkeyhint="send"
            autocomplete="off"
            aria-label="Terminal input"
            class="flex-1 border-none outline-none bg-transparent font-mono text-small leading-5 p-0"
          />
        </form>
      </div>
      <Show when={M.mobile}>
        <KeyBar terminal={props.terminal} focusInput={focusInput} />
      </Show>
    </>
  );
}
