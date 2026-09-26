import type { TerminalKey } from "@marshal/protocol";
import { Icon } from "@marshal/ui";
import { Index, Show } from "solid-js";
import { isDaemon } from "~/data/sections";
import { type Card, M } from "~/mock";
import type { TerminalLine } from "./terminal-lines";
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

/** The key bar's own key names, mapped to the wire's named keys (docs/architecture.md 11.2), for
 * the daemon path: the bar has no Enter, Backspace, Delete, Home, End, Page Up, or Page Down
 * buttons today, so only what it already offers is sent, and nothing new is added for it. `Ctrl`
 * is not here: it only arms the bar's own toggle (see `press` below), and none of the keys it can
 * be combined with with has a named `Ctrl+`-something in the wire's own `TerminalKey` (only
 * `ctrl-c`, `ctrl-d`, `ctrl-l`, and `ctrl-z`, none of them a key this bar offers), so arming it and
 * pressing one of these still sends the key alone. */
const TERM_KEY_MAP: Readonly<Record<string, TerminalKey>> = {
  Esc: "esc",
  Tab: "tab",
  "arrow-left": "left",
  "arrow-up": "up",
  "arrow-down": "down",
  "arrow-right": "right",
};

/** The phone's key bar: keys a touch keyboard lacks. Pressed keys are logged (the mock path), and,
 * when `onWireKey` is given (the daemon path), also sent into the card's real terminal. */
function KeyBar(props: {
  terminal: TerminalState;
  focusInput: () => void;
  onWireKey?: (key: TerminalKey) => void;
}) {
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
                if (item().key === "Ctrl") return;
                const wireKey = TERM_KEY_MAP[item().key];
                if (wireKey) props.onWireKey?.(wireKey);
                props.focusInput();
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

/** A daemon's own terminal, as decoded, escape-stripped plain text (section S9): true only for a
 * card that is the daemon's own and while S9 itself is switched, so a mock-only card, or one
 * opened while S9 is still on the mock (`install-test-store.ts`'s shared test store, most
 * component tests), keeps the fake terminal exactly as it always drew it. */
const isDaemonTerminal = (card: Card): boolean => isDaemon("S9") && !!card.daemonId;

/** The daemon's own text, split into lines for the same `<Index>` the mock's fake lines use, so
 * the container, the font, and the layout are untouched: only the source of the text changes. */
function daemonLines(card: Card): TerminalLine[] {
  return M.terminalText(card.id)
    .split("\n")
    .map((text) => ({ text, class: "text-primary" }));
}

/** The terminal mode: the session as plain text with a prompt. For a mock-only card, or while S9
 * itself is still the mock's, it is fake, as in the design; for a real card once S9 is switched, it
 * is the daemon's own terminal output, decoded and stripped of escape sequences (not a terminal
 * emulator: no cursor positioning, no alternate screens — a deliberate simplification, see
 * `sync/card-view.ts`). */
export function TerminalView(props: TerminalViewProps) {
  let input: HTMLInputElement | undefined;
  const focusInput = () => input?.focus();
  const daemon = () => isDaemonTerminal(props.card);
  const lines = () =>
    daemon()
      ? daemonLines(props.card)
      : terminalLines(props.card, M.S.chat[props.card.id] ?? [], props.terminal.keyLog());
  const submit = (e: SubmitEvent & { currentTarget: HTMLFormElement }) => {
    e.preventDefault();
    const field = e.currentTarget.elements.namedItem("cmd");
    if (!(field instanceof HTMLInputElement)) return;
    if (daemon()) {
      // Not trimmed, and sent even when empty: the bar has no Enter key, so a bare Enter (this
      // view's own way of answering a program's "press Enter" prompt) is submitting an empty field.
      M.terminalSend(props.card.id, field.value);
      field.value = "";
      return;
    }
    if (!field.value.trim()) return;
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
        <KeyBar
          terminal={props.terminal}
          focusInput={focusInput}
          onWireKey={daemon() ? (key) => M.terminalKey(props.card.id, key) : undefined}
        />
      </Show>
    </>
  );
}
