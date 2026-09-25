import { Input, type InputProps } from "@marshal/ui";
import { createEffect, splitProps } from "solid-js";

export interface NumberInputProps extends Omit<InputProps, "type" | "value" | "ref"> {
  value: number;
}

/**
 * A number field whose text follows a number. It only rewrites the text when the typed number
 * differs, so a half-typed "0." or "1e" is not replaced while the draft holds 0 or 1 (React does
 * the same for controlled number inputs, which the design relies on).
 */
export function NumberInput(props: NumberInputProps) {
  const [local, others] = splitProps(props, ["value"]);
  let field: HTMLInputElement | undefined;
  createEffect(() => {
    const value = local.value;
    if (!field || Number.isNaN(value)) return;
    const typed = field.value === "" ? Number.NaN : Number(field.value);
    if (typed !== value) field.value = String(value);
  });
  return (
    <Input
      {...others}
      type="number"
      ref={(el) => {
        field = el;
      }}
    />
  );
}
