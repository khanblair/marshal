import { type JSX, Match, Switch, splitProps } from "solid-js";
import { Dynamic } from "solid-js/web";
import { cx } from "../base/cx";
import { type IconNameInput, resolveIcon } from "./icon-names";
import { lucideIcons } from "./lucide-icons";
import { statusGlyphs } from "./status-glyphs";

const DEFAULT_ICON_SIZE_PX = 16;
/** Icons at or below this size get the heavier stroke, as in the design. */
const SMALL_ICON_MAX_PX = 14;
const SMALL_ICON_STROKE_PX = 1.75;
const ICON_STROKE_PX = 1.5;
const VIEWBOX_SIZE = 24;
const STROKE_SCALE = 0.85;
const DECIMALS = 2;
/** The spinner ring is three quarters of the icon box. */
const SPINNER_RATIO = 0.75;

/**
 * Stroke width in viewBox units, so the drawn stroke stays the same number of
 * pixels at every size. Same formula and operation order as the prototype.
 */
export function iconStrokeWidth(size: number): string {
  const stroke = size <= SMALL_ICON_MAX_PX ? SMALL_ICON_STROKE_PX : ICON_STROKE_PX;
  return (((stroke * VIEWBOX_SIZE) / size) * STROKE_SCALE).toFixed(DECIMALS);
}

export interface IconProps extends Omit<JSX.HTMLAttributes<HTMLSpanElement>, "children" | "style"> {
  /** Kebab-case name from the design, for example `file-search` or `st-working`. */
  name: IconNameInput;
  /** Box size in px. Default 16. */
  size?: number;
  /** Extra inline styles, such as a status color that comes from data. */
  style?: JSX.CSSProperties;
}

function Frame(props: { size: number; children?: JSX.Element }) {
  return (
    <svg
      aria-hidden="true"
      xmlns="http://www.w3.org/2000/svg"
      width={props.size}
      height={props.size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width={iconStrokeWidth(props.size)}
      stroke-linecap="round"
      stroke-linejoin="round"
      class="block"
    >
      {props.children}
    </svg>
  );
}

function Glyph(props: { name: string; size: number }) {
  const resolved = () => resolveIcon(props.name);
  const glyph = () => {
    const icon = resolved();
    return icon.kind === "glyph" ? icon.name : undefined;
  };
  const lucide = () => {
    const icon = resolved();
    return icon.kind === "lucide" ? icon.name : undefined;
  };
  const ring = () => `${Math.round(props.size * SPINNER_RATIO)}px`;
  return (
    <Switch fallback={<Frame size={props.size} />}>
      <Match when={resolved().kind === "spinner"}>
        <span
          class="block box-border rounded-full border-[1.5px] border-current border-r-transparent animate-spin-fast"
          style={{ width: ring(), height: ring() }}
        />
      </Match>
      <Match when={glyph()}>
        {(name) => <Frame size={props.size}>{statusGlyphs[name()]()}</Frame>}
      </Match>
      <Match when={lucide()}>
        {(name) => (
          <Dynamic
            component={lucideIcons[name()]}
            size={props.size}
            stroke-width={iconStrokeWidth(props.size)}
            class="block"
          />
        )}
      </Match>
    </Switch>
  );
}

/**
 * An icon from the design's set: Lucide icons, the status flags, and `spinner`.
 * Port of the prototype's `m-icon`. It is decorative (`aria-hidden`), so give
 * the control around it an accessible name.
 */
export function Icon(props: IconProps) {
  const [local, others] = splitProps(props, ["name", "size", "class", "style"]);
  const size = () => local.size ?? DEFAULT_ICON_SIZE_PX;
  return (
    <span
      aria-hidden="true"
      {...others}
      class={cx("inline-flex flex-none items-center justify-center leading-[0]", local.class)}
      style={{ ...local.style, width: `${size()}px`, height: `${size()}px` }}
    >
      <Glyph name={local.name} size={size()} />
    </span>
  );
}
