import { describe, expect, it } from "vitest";
import { fieldValue } from "./form-field";

function formWith(html: string): HTMLFormElement {
  const form = document.createElement("form");
  form.innerHTML = html;
  return form;
}

describe("fieldValue", () => {
  it("reads inputs, selects, and text areas by name", () => {
    const form = formWith(`
      <input name="title" value="Morning brief">
      <select name="trigger"><option>Cron</option><option selected>Interval</option></select>
      <textarea name="notes">two lines</textarea>`);
    expect(fieldValue(form, "title")).toBe("Morning brief");
    expect(fieldValue(form, "trigger")).toBe("Interval");
    expect(fieldValue(form, "notes")).toBe("two lines");
  });

  it("gives an empty string for a name the form does not have", () => {
    expect(fieldValue(formWith('<input name="a">'), "missing")).toBe("");
  });
});
