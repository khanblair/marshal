import { Icon } from "@marshal/ui";

const SEARCH_ICON_PX = 16;
const SEARCH_LABEL = "Search actions, projects, cards, and settings";

export interface PaletteSearchProps {
  value: string;
  onInput: (value: string) => void;
  onKeyDown: (event: KeyboardEvent) => void;
  onClose: () => void;
  /** Receives the input, so the palette can focus it. */
  inputRef: (el: HTMLInputElement) => void;
}

/** The palette's top row: the search icon, the field, and the Esc button. */
export function PaletteSearch(props: PaletteSearchProps) {
  return (
    <div class="flex items-center gap-2.5 pl-4 pr-3 min-h-13 border-b border-border">
      <Icon name="search" size={SEARCH_ICON_PX} />
      <input
        ref={props.inputRef}
        value={props.value}
        onInput={(e) => props.onInput(e.currentTarget.value)}
        onKeyDown={props.onKeyDown}
        placeholder={SEARCH_LABEL}
        aria-label={SEARCH_LABEL}
        class="flex-1 min-w-0 h-10 border-none bg-transparent text-subtitle outline-none"
      />
      <button
        type="button"
        onClick={() => props.onClose()}
        aria-label="Close search"
        class="h-7 px-1.5 border border-border rounded-xs bg-transparent text-caption text-muted"
      >
        Esc
      </button>
    </div>
  );
}
