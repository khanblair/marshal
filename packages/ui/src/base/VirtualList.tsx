import {
  type Accessor,
  createEffect,
  createMemo,
  createSignal,
  For,
  type JSX,
  on,
  onCleanup,
  onMount,
  Show,
  splitProps,
} from "solid-js";
import { cx } from "./cx";
import {
  listHeight,
  rowOffsets,
  rowsBetween,
  rowsToDraw,
  rowWindow,
  sameRange,
  scrollTopOfRow,
} from "./virtual-window";

export interface VirtualListHandle {
  /** Scrolls the list so the row is at the top of the screen. */
  scrollToIndex: (index: number) => void;
}

export interface VirtualListProps<T>
  extends Omit<JSX.HTMLAttributes<HTMLDivElement>, "children" | "ref" | "onScroll"> {
  items: readonly T[];
  /** A name for an item that stays the same when the list changes, to remember its measured height. */
  itemKey: (item: T) => string;
  /**
   * The height in px a row is given before it is measured, and after its look changes (an open
   * file is taller than a closed one). Read inside the list's own effects, so a signal it reads
   * moves the rows when it changes.
   */
  estimateHeight: (item: T) => number;
  /** Space between rows in px. Default 0. */
  gap?: number;
  /** Extra px drawn above and below the screen. Default 400. */
  overscan?: number;
  /** A name for the list, for a screen reader. */
  label?: string;
  /** Content above the rows that scrolls with them. */
  before?: JSX.Element;
  /** Called once with the list's own controls. */
  handle?: (handle: VirtualListHandle) => void;
  /** Draws one row. `item` follows the list when it is replaced, and `index` is the row's place. */
  children: (item: Accessor<T>, index: number) => JSX.Element;
}

interface Measured {
  /** What `estimateHeight` said when the row was measured: a different estimate means the row changed look. */
  estimate: number;
  height: number;
}

const DEFAULT_OVERSCAN_PX = 400;
/** A row that measures within this many px of what the list already assumes is left alone. */
const SAME_HEIGHT_PX = 0.5;

const observer = (callback: ResizeObserverCallback): ResizeObserver | undefined =>
  typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(callback);

const blockSize = (entry: ResizeObserverEntry): number =>
  entry.borderBoxSize?.[0]?.blockSize ?? (entry.target as HTMLElement).offsetHeight;

const indexOfRow = (el: Element): number => Number((el as HTMLElement).dataset.index);

/** The row a focus event happened in, by its index, or null when it was outside every row. */
function rowOfEvent(event: FocusEvent): number | null {
  const row = (event.target as HTMLElement).closest<HTMLElement>("[data-index]");
  return row ? indexOfRow(row) : null;
}

type LayoutProps<T> = Pick<
  VirtualListProps<T>,
  "items" | "itemKey" | "estimateHeight" | "gap" | "overscan"
>;

interface RowLayout {
  offsets: Accessor<number[]>;
  /** The rows to draw now, in order. */
  drawn: Accessor<number[]>;
  /** Takes the measures of drawn rows. */
  resized: (entries: ResizeObserverEntry[]) => void;
}

/**
 * Where every row is: its measured height where it has one, else its estimate, and from them
 * the rows to draw. `scrollTop` is the position in the list, `viewport` the height of the screen,
 * and `focused` the row that must stay drawn.
 */
function createRowLayout<T>(
  props: LayoutProps<T>,
  view: {
    scrollTop: Accessor<number>;
    viewport: Accessor<number>;
    focused: Accessor<number | null>;
  },
): RowLayout {
  /** Counts the measurements, so the heights are worked out again. */
  const [revision, setRevision] = createSignal(0);
  const measured = new Map<string, Measured>();
  const gap = () => props.gap ?? 0;
  const heightOf = (item: T): number => {
    const estimate = props.estimateHeight(item);
    const seen = measured.get(props.itemKey(item));
    return seen && seen.estimate === estimate ? seen.height : estimate;
  };
  const heights = createMemo(() => {
    revision();
    return props.items.map(heightOf);
  });
  const offsets = createMemo(() => rowOffsets(heights(), gap()));
  const range = createMemo(
    () =>
      rowWindow({
        offsets: offsets(),
        scrollTop: view.scrollTop(),
        viewportHeight: view.viewport(),
        overscan: props.overscan ?? DEFAULT_OVERSCAN_PX,
      }),
    undefined,
    { equals: sameRange },
  );
  const drawn = createMemo(() => rowsToDraw(range(), view.focused(), props.items.length));

  /**
   * Remembers a drawn row's real height. Says whether it changed anything: a row that measures as
   * it was assumed to be, or that cannot be measured (a hidden tab measures 0, which is not a
   * height), is left alone.
   */
  const measure = (entry: ResizeObserverEntry): boolean => {
    const index = indexOfRow(entry.target);
    const item = props.items[index];
    const height = blockSize(entry);
    const known = heights()[index] ?? height;
    if (item === undefined || height <= 0 || Math.abs(height - known) < SAME_HEIGHT_PX)
      return false;
    measured.set(props.itemKey(item), { estimate: props.estimateHeight(item), height });
    return true;
  };
  const resized = (entries: ResizeObserverEntry[]) => {
    const changed = entries.map(measure).some(Boolean);
    if (changed) setRevision((count) => count + 1);
  };
  return { offsets, drawn, resized };
}

interface Anchor {
  key: string;
  /** How far into the row the top of the screen was. */
  inset: number;
}

/**
 * Keeps the row at the top of the screen where it is when the rows above it change height, which
 * happens when one is measured, opened, or closed. `remember` notes the row for a scroll position,
 * and `shift` says how far to scroll now to put that row back where it was.
 */
function createAnchor<T>(
  props: Pick<VirtualListProps<T>, "items" | "itemKey">,
  offsets: Accessor<number[]>,
) {
  let anchor: Anchor | undefined;
  return {
    remember: (scrollTop: number) => {
      const { first } = rowsBetween(offsets(), scrollTop, scrollTop);
      const item = props.items[first];
      const start = offsets()[first] ?? 0;
      anchor =
        item === undefined ? undefined : { key: props.itemKey(item), inset: scrollTop - start };
    },
    shift: (scrollTop: number): number => {
      const kept = anchor;
      if (!kept) return 0;
      const index = props.items.findIndex((item) => props.itemKey(item) === kept.key);
      return index < 0 ? 0 : (offsets()[index] ?? 0) + kept.inset - scrollTop;
    },
  };
}

interface VirtualState {
  layout: RowLayout;
  /** Reads the scroller's position and height again. */
  sync: () => void;
  setScroller: (el: HTMLDivElement) => void;
  setList: (el: HTMLUListElement) => void;
  /** A focus event in the list: the row it happened in, or null when focus left the list. */
  setFocused: (row: number | null) => void;
  contains: (node: Node | null) => boolean;
  /** Watches a drawn row's size. */
  observeRow: (el: HTMLElement) => void;
  scrollToIndex: (index: number) => void;
}

/**
 * Follows the scroller (how far it is scrolled, how tall it is) and the rows' layout that comes
 * from it, and keeps the row at the top of the screen still when rows above it change height.
 */
function createVirtualState<T>(props: LayoutProps<T>): VirtualState {
  let scroller: HTMLDivElement | undefined;
  let listEl: HTMLUListElement | undefined;
  /** How far the list is scrolled: the scroller's own position less the space above the list. */
  const [scrollTop, setScrollTop] = createSignal(0);
  const [viewport, setViewport] = createSignal(0);
  const [focused, setFocused] = createSignal<number | null>(null);
  const layout = createRowLayout(props, { scrollTop, viewport, focused });
  const anchor = createAnchor(props, layout.offsets);
  const sync = () => {
    if (!scroller) return;
    const top = scroller.scrollTop - (listEl?.offsetTop ?? 0);
    setScrollTop(top);
    setViewport(scroller.clientHeight);
    anchor.remember(top);
  };
  const rowObserver = observer(layout.resized);
  onCleanup(() => rowObserver?.disconnect());
  // Rows above the screen that change height would push what is on it down or up. Scrolling by as
  // much keeps it still. This runs before the effect below, which notes the new position.
  createEffect(
    on(
      layout.offsets,
      () => {
        const shift = anchor.shift(scrollTop());
        if (Math.abs(shift) < SAME_HEIGHT_PX || !scroller) return;
        scroller.scrollTop += shift;
        sync();
      },
      { defer: true },
    ),
  );
  onMount(() => {
    const viewObserver = observer(sync);
    if (scroller) viewObserver?.observe(scroller);
    onCleanup(() => viewObserver?.disconnect());
  });
  // The space above the list changes with what `before` holds and with the list itself, and none
  // of that scrolls anything, so the position is read again whenever the items change.
  createEffect(on(() => props.items, sync));
  return {
    layout,
    sync,
    setScroller: (el) => {
      scroller = el;
    },
    setList: (el) => {
      listEl = el;
    },
    setFocused,
    contains: (node) => !!listEl?.contains(node),
    observeRow: (el) => {
      rowObserver?.observe(el);
      onCleanup(() => rowObserver?.unobserve(el));
    },
    scrollToIndex: (index) => {
      if (!scroller) return;
      scroller.scrollTop = (listEl?.offsetTop ?? 0) + scrollTopOfRow(layout.offsets(), index);
    },
  };
}

interface RowProps<T> {
  index: number;
  items: readonly T[];
  top: number;
  observe: (el: HTMLElement) => void;
  children: VirtualListProps<T>["children"];
}

/** One drawn row: a list item that says where it is in the list, and follows its item. */
function VirtualRow<T>(props: RowProps<T>) {
  // A row can outlive the item it drew for one update, when the list gets shorter.
  const item = createMemo((previous: T | undefined) =>
    props.index < props.items.length ? (props.items[props.index] as T) : previous,
  ) as Accessor<T>;
  return (
    <li
      data-index={props.index}
      aria-posinset={props.index + 1}
      aria-setsize={props.items.length}
      class="absolute inset-x-0"
      style={{ top: `${props.top}px` }}
      ref={props.observe}
    >
      {props.children(item, props.index)}
    </li>
  );
}

/**
 * A tall list that draws only the rows near the screen: the visible ones, and some overscan above
 * and below. Rows can have different heights, so each drawn row is measured, and until then it is
 * given `estimateHeight`. The list is its own scroller, and `before` scrolls with the rows.
 *
 * The row that has focus stays drawn when scrolled away, so a person using the keyboard does not
 * lose their place, and each row says where it is in the list for a screen reader.
 */
export function VirtualList<T>(props: VirtualListProps<T>) {
  const [local, others] = splitProps(props, [
    "items",
    "itemKey",
    "estimateHeight",
    "gap",
    "overscan",
    "label",
    "before",
    "handle",
    "children",
    "class",
  ]);
  const state = createVirtualState(local);
  local.handle?.({ scrollToIndex: state.scrollToIndex });
  const { offsets, drawn } = state.layout;
  return (
    <div
      {...others}
      ref={state.setScroller}
      onScroll={state.sync}
      class={cx("relative overflow-auto [overflow-anchor:none]", local.class)}
    >
      {local.before}
      <Show when={local.items.length > 0}>
        <ul
          ref={state.setList}
          aria-label={local.label}
          onFocusIn={(event) => state.setFocused(rowOfEvent(event))}
          onFocusOut={(event) => {
            if (!state.contains(event.relatedTarget as Node | null)) state.setFocused(null);
          }}
          class="relative flex-none m-0 p-0 list-none"
          style={{ height: `${listHeight(offsets(), local.gap ?? 0)}px` }}
        >
          <For each={drawn()}>
            {(index) => (
              <VirtualRow
                index={index}
                items={local.items}
                top={offsets()[index] ?? 0}
                observe={state.observeRow}
              >
                {local.children}
              </VirtualRow>
            )}
          </For>
        </ul>
      </Show>
    </div>
  );
}
