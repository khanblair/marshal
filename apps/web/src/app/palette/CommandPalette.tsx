import { cx, FOCUS_DELAY_MS, Scrim } from "@marshal/ui";
import {
  createEffect,
  createMemo,
  createSignal,
  Index,
  on,
  onCleanup,
  onMount,
  Show,
} from "solid-js";
import { M } from "~/mock";
import { modKey } from "../shell-layout";
import { PaletteOption } from "./PaletteOption";
import { PaletteSearch } from "./PaletteSearch";
import {
  type PaletteCommand,
  paletteResults,
  scrollOptionIntoView,
  stepSelection,
} from "./palette-results";

const closePalette = (): void => M.set({ palette: false });

function runCommand(command: PaletteCommand): void {
  closePalette();
  command.run();
}

/** Focuses the search field once the palette is painted, and gives focus back on close. */
function useFocusReturn(input: () => HTMLInputElement | undefined): void {
  const opener = document.activeElement as HTMLElement | null;
  onMount(() => {
    const timer = setTimeout(() => input()?.focus(), FOCUS_DELAY_MS);
    onCleanup(() => clearTimeout(timer));
  });
  onCleanup(() => opener?.focus?.());
}

function PaletteBody() {
  let input: HTMLInputElement | undefined;
  let list: HTMLDivElement | undefined;
  const [query, setQuery] = createSignal("");
  const [sel, setSel] = createSignal(0);
  // The daemon's answer to the typed query, when there is one. Until it lands, and whenever there is
  // none, the palette searches what the store holds. The session stops when the palette closes.
  const search = M.startSearch();
  const results = createMemo(() => paletteResults(M, query(), search.hits()));
  const shown = () => Math.min(sel(), Math.max(0, results().length - 1));
  useFocusReturn(() => input);
  // A new answer changes the rows, so the selection goes back to the top, as it does while typing.
  createEffect(on(search.hits, () => setSel(0), { defer: true }));

  const onKeyDown = (e: KeyboardEvent): void => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const next = stepSelection(shown(), e.key, results().length);
      setSel(next);
      setTimeout(() => list && scrollOptionIntoView(list, next), 0);
    }
    const chosen = results()[shown()];
    if (e.key === "Enter" && chosen) {
      e.preventDefault();
      runCommand(chosen);
    }
  };

  return (
    <>
      <Scrim tone="sheet" class="z-[399]" onClick={closePalette} />
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
        class={cx(
          "absolute z-palette flex flex-col bg-surface-raised",
          M.mobile
            ? "inset-0 pt-[env(safe-area-inset-top)]"
            : "left-1/2 top-[12%] -translate-x-1/2 w-[min(640px,calc(100%-24px))] max-h-[70%] rounded-xl border border-border shadow-e2 overflow-hidden",
        )}
      >
        <PaletteSearch
          value={query()}
          onInput={(value) => {
            setQuery(value);
            setSel(0);
            search.ask(value);
          }}
          onKeyDown={onKeyDown}
          onClose={closePalette}
          inputRef={(el) => {
            input = el;
          }}
        />
        <div ref={list} role="listbox" class="flex-1 min-h-0 overflow-auto p-1.5">
          <Show when={results().length === 0}>
            <p class="m-6 text-center text-secondary">No results for "{query()}".</p>
          </Show>
          <Index each={results()}>
            {(item, i) => (
              <PaletteOption
                item={item()}
                index={i}
                showGroup={i === 0 || results()[i - 1]?.group !== item().group}
                selected={i === shown()}
                phone={M.mobile}
                modKey={modKey()}
                onHover={() => sel() !== i && setSel(i)}
                onRun={() => runCommand(item())}
              />
            )}
          </Index>
        </div>
      </div>
    </>
  );
}

/**
 * The command palette. Its query and selection live only while it is open, so they reset on
 * every open. Renders nothing while `M.S.palette` is off.
 */
export function CommandPalette() {
  return (
    <Show when={M.S.palette}>
      <PaletteBody />
    </Show>
  );
}
