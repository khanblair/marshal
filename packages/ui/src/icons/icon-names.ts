import type { LooseString } from "../base/types";
import { type LucideIconName, lucideIcons } from "./lucide-icons";
import { type StatusGlyphName, statusGlyphs } from "./status-glyphs";

/** Status names that reuse a Lucide icon (the ALIAS table in store.js). */
export const iconAliases = {
  "st-needs": "hand",
  "st-review": "eye",
  "st-ready": "git-merge",
  "st-merging": "git-merge",
} as const satisfies Record<string, LucideIconName>;

type IconAlias = keyof typeof iconAliases;

/** The loading spinner, drawn with CSS instead of an SVG path. */
export const SPINNER_ICON = "spinner";

/** Every icon name the design uses. */
export type IconName = LucideIconName | StatusGlyphName | IconAlias | typeof SPINNER_ICON;

/** An icon name, or any string from data. Unknown names draw an empty icon. */
export type IconNameInput = IconName | LooseString;

/** How an icon name is drawn. */
export type ResolvedIcon =
  | { kind: "spinner" }
  | { kind: "glyph"; name: StatusGlyphName }
  | { kind: "lucide"; name: LucideIconName }
  | { kind: "unknown" };

const has = (table: object, key: string) => Object.hasOwn(table, key);

export function resolveIcon(name: string): ResolvedIcon {
  if (name === SPINNER_ICON) return { kind: "spinner" };
  if (has(statusGlyphs, name)) return { kind: "glyph", name: name as StatusGlyphName };
  if (has(iconAliases, name)) return { kind: "lucide", name: iconAliases[name as IconAlias] };
  if (has(lucideIcons, name)) return { kind: "lucide", name: name as LucideIconName };
  return { kind: "unknown" };
}

export function isIconName(name: string): name is IconName {
  return resolveIcon(name).kind !== "unknown";
}

/** All icon names, sorted. */
export const iconNames: readonly IconName[] = (
  [
    ...Object.keys(lucideIcons),
    ...Object.keys(statusGlyphs),
    ...Object.keys(iconAliases),
    SPINNER_ICON,
  ] as IconName[]
).sort();
