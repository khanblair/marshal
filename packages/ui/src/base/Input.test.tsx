import { fireEvent, render, screen } from "@solidjs/testing-library";
import { Input } from "./Input";

describe("Input", () => {
  it("is a 32 px field with a strong border", () => {
    render(() => <Input aria-label="Name" placeholder="Optional" />);
    const input = screen.getByRole("textbox", { name: "Name" });
    expect(input).toHaveAttribute("placeholder", "Optional");
    expect(input).toHaveClass(
      "h-8",
      "px-2.5",
      "rounded-sm",
      "border",
      "border-border-strong",
      "bg-surface",
    );
    expect(input).not.toHaveAttribute("aria-invalid");
  });

  it("marks invalid fields and draws monospace text", () => {
    render(() => <Input aria-label="Key" invalid mono class="flex-1" />);
    const input = screen.getByRole("textbox");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveClass("border-status-danger-solid", "font-mono", "text-small", "flex-1");
    expect(input).not.toHaveClass("border-border-strong");
  });

  it("passes native events and refs through", () => {
    const onInput = vi.fn(
      (event: InputEvent & { currentTarget: HTMLInputElement }) => event.currentTarget.value,
    );
    let ref: HTMLInputElement | undefined;
    render(() => <Input aria-label="Title" onInput={onInput} ref={ref} value="a" />);
    const input = screen.getByRole("textbox") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "ab" } });
    expect(onInput).toHaveReturnedWith("ab");
    expect(ref).toBe(input);
  });
});
