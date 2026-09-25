type FieldElement = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;

const isField = (el: unknown): el is FieldElement =>
  el instanceof HTMLInputElement ||
  el instanceof HTMLSelectElement ||
  el instanceof HTMLTextAreaElement;

/** The current value of the control called `name` in a form; an empty string when it is not there. */
export function fieldValue(form: HTMLFormElement, name: string): string {
  const el = form.elements.namedItem(name);
  return isField(el) ? el.value : "";
}
