import { createSignal } from "solid-js";

/**
 * The terminal's Ctrl toggle and the keys pressed on the phone key bar. They live above
 * the per-card panel, so switching cards keeps them, as the design does.
 */
export interface TerminalState {
  ctrl: () => boolean;
  keyLog: () => readonly string[];
  /** A key of the bar was pressed: Ctrl toggles, any other key is logged (with Ctrl+ when armed). */
  press: (key: string, label: string) => void;
}

export function createTerminalState(): TerminalState {
  const [ctrl, setCtrl] = createSignal(false);
  const [keyLog, setKeyLog] = createSignal<readonly string[]>([]);
  return {
    ctrl,
    keyLog,
    press: (key, label) => {
      if (key === "Ctrl") {
        setCtrl(!ctrl());
        return;
      }
      setKeyLog([...keyLog(), `${ctrl() ? "Ctrl+" : ""}${label}`]);
      setCtrl(false);
    },
  };
}
