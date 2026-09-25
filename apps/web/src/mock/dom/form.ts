/** Value of a named field in the form that fired `e`, like the prototype's `e.target.steps.value`. */
export function formValue(e: Event, name: string): string {
  const form = e.target;
  if (!(form instanceof HTMLFormElement)) return "";
  const field = form.elements.namedItem(name);
  if (field instanceof HTMLTextAreaElement || field instanceof HTMLInputElement) return field.value;
  return "";
}
