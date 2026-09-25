import { fireEvent, render, screen } from "@solidjs/testing-library";
import { Checkbox } from "./Checkbox";

describe("Checkbox", () => {
  it("is an 18 px native checkbox in the ink accent", () => {
    const onChange = vi.fn();
    render(() => <Checkbox aria-label="Start the card right away" onChange={onChange} />);
    const box = screen.getByRole("checkbox", { name: "Start the card right away" });
    expect(box).toHaveClass("size-4.5", "accent-ink", "mt-0", "mx-0", "mb-0");
    fireEvent.click(box);
    expect(onChange).toHaveBeenCalledOnce();
    expect(box).toBeChecked();
  });

  it("can be 16 px, danger colored, and aligned to the first text line", () => {
    render(() => (
      <Checkbox aria-label="I understand" size={16} tone="danger" align="start" checked />
    ));
    const box = screen.getByRole("checkbox");
    expect(box).toHaveClass("size-4", "accent-status-danger-solid", "mt-px");
    expect(box).toBeChecked();
  });
});
