import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { createSignal } from "solid-js";
import { afterEach, describe, expect, it } from "vitest";
import { NumberInput } from "./NumberInput";

afterEach(cleanup);

describe("NumberInput", () => {
  it("shows its number and follows it when it changes", () => {
    const [value, setValue] = createSignal(60);
    render(() => <NumberInput aria-label="Time limit" value={value()} />);
    const field = screen.getByLabelText("Time limit");
    expect(field).toHaveAttribute("type", "number");
    expect(field).toHaveValue(60);
    setValue(90);
    expect(field).toHaveValue(90);
  });

  it("does not rewrite what was typed while it means the same number", () => {
    const [value, setValue] = createSignal(3);
    render(() => (
      <NumberInput
        aria-label="Cost limit"
        value={value()}
        onInput={(e) => setValue(+e.currentTarget.value)}
      />
    ));
    const field = screen.getByLabelText<HTMLInputElement>("Cost limit");
    fireEvent.input(field, { target: { value: "3.50" } });
    expect(value()).toBe(3.5);
    expect(field.value).toBe("3.50");
  });

  it("puts a number back when the text was cleared", () => {
    const [value, setValue] = createSignal(5);
    render(() => <NumberInput aria-label="Round limit" value={value()} />);
    const field = screen.getByLabelText<HTMLInputElement>("Round limit");
    field.value = "";
    setValue(6);
    expect(field.value).toBe("6");
  });

  it("leaves an empty field alone while the value is not a number", () => {
    const [value, setValue] = createSignal(Number.NaN);
    render(() => <NumberInput aria-label="Limit" value={value()} />);
    const field = screen.getByLabelText<HTMLInputElement>("Limit");
    expect(field.value).toBe("");
    setValue(Number.NaN);
    expect(field.value).toBe("");
  });
});
