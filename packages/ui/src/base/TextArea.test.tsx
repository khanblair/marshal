import { fireEvent, render, screen } from "@solidjs/testing-library";
import { TextArea } from "./TextArea";

describe("TextArea", () => {
  it("is a bordered field that resizes vertically", () => {
    const onInput = vi.fn();
    render(() => <TextArea aria-label="Description" rows={3} onInput={onInput} />);
    const area = screen.getByRole("textbox", { name: "Description" });
    expect(area).toHaveAttribute("rows", "3");
    expect(area).toHaveClass("py-2", "px-2.5", "rounded-sm", "border-border-strong", "resize-y");
    fireEvent.input(area, { target: { value: "x" } });
    expect(onInput).toHaveBeenCalledOnce();
  });

  it("draws the monospace note style and the invalid border", () => {
    render(() => <TextArea aria-label="Card note" mono invalid />);
    const area = screen.getByRole("textbox");
    expect(area).toHaveClass(
      "p-2.5",
      "font-mono",
      "text-small",
      "leading-5",
      "border-status-danger-solid",
    );
    expect(area).toHaveAttribute("aria-invalid", "true");
  });
});
