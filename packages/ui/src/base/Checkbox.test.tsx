import { fireEvent, render, screen } from "@solidjs/testing-library";
import { Checkbox } from "./Checkbox";

describe("Checkbox", () => {
  it("is an 18 px drawn box that is still a native checkbox", () => {
    const onChange = vi.fn();
    render(() => <Checkbox aria-label="Start the card right away" onChange={onChange} />);
    const box = screen.getByRole("checkbox", { name: "Start the card right away" });
    expect(box).toHaveClass("size-4.5", "appearance-none", "m-0", "mt-0", "border-border-strong");
    expect(box).toHaveClass("checked:bg-ink", "checked:border-ink", "before:border-on-ink");
    fireEvent.click(box);
    expect(onChange).toHaveBeenCalledOnce();
    expect(box).toBeChecked();
  });

  it("can be 16 px, danger colored, and aligned to the first text line", () => {
    render(() => (
      <Checkbox aria-label="I understand" size={16} tone="danger" align="start" checked />
    ));
    const box = screen.getByRole("checkbox");
    expect(box).toHaveClass(
      "size-4",
      "checked:bg-status-danger-solid",
      "before:border-white",
      "mt-px",
    );
    expect(box).toBeChecked();
  });

  it("is dimmed when disabled, and takes no color from the browser's own accent", () => {
    render(() => <Checkbox aria-label="Off" disabled />);
    const box = screen.getByRole("checkbox");
    expect(box).toHaveClass("disabled:opacity-50");
    expect(box.className).not.toContain("accent-");
  });
});
