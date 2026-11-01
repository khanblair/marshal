import { fireEvent } from "@solidjs/testing-library";

/**
 * The dropdowns are Marshal's own (packages/ui Select), not native selects, so a test opens one and
 * presses an option. `trigger` is the combobox button a test found by its role or its label.
 */
export function pickValue(trigger: HTMLElement, value: string): void {
  fireEvent.click(trigger);
  const option = document.querySelector<HTMLElement>(
    `[role="menuitemradio"][data-value="${value}"]`,
  );
  if (!option) throw new Error(`the dropdown has no option with the value ${value}`);
  fireEvent.click(option);
}

/** The value a dropdown holds, read from the field it keeps for forms. */
export function selectedValue(trigger: HTMLElement): string {
  const field = trigger.parentElement?.querySelector<HTMLInputElement>('input[type="hidden"]');
  if (!field) throw new Error("this is not a dropdown");
  return field.value;
}

/** The options a dropdown lists. It is opened to read them and closed again. */
export function optionsOf(trigger: HTMLElement): HTMLElement[] {
  fireEvent.click(trigger);
  const options = [...document.querySelectorAll<HTMLElement>('[role="menuitemradio"]')];
  fireEvent.click(trigger);
  return options;
}
