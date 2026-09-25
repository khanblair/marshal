/**
 * Joins class names, skipping empty values.
 *
 * There is no class merging: when two classes set the same property, the CSS
 * order decides, not the order here. To override a class a component sets, add
 * Tailwind's important suffix, for example `px-4!`.
 */
export function cx(...classes: (string | false | null | undefined)[]): string {
  return classes.filter(Boolean).join(" ");
}
