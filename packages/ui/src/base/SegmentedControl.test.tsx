import { fireEvent, render, screen } from "@solidjs/testing-library";
import { createSignal } from "solid-js";
import { SegmentedControl, type SegmentOption } from "./SegmentedControl";

type Range = "7" | "30" | "90";
const ranges: SegmentOption<Range>[] = [
  { value: "7", label: "7 days" },
  { value: "30", label: "30 days" },
  { value: "90", label: "90 days" },
];

function Ranges(props: { onChange?: (value: Range) => void; disabled?: boolean }) {
  const [value, setValue] = createSignal<Range>("7");
  const options = () => ranges.map((o) => ({ ...o, disabled: props.disabled && o.value === "30" }));
  return (
    <SegmentedControl
      label="Chart range"
      options={options()}
      value={value()}
      onValueChange={(next) => {
        setValue(next);
        props.onChange?.(next);
      }}
    />
  );
}

describe("SegmentedControl", () => {
  it("is a radiogroup of radios with the selected one raised", () => {
    render(() => <Ranges />);
    expect(screen.getByRole("radiogroup", { name: "Chart range" })).toHaveClass(
      "bg-surface-sunken",
      "rounded-md",
      "p-0.5",
    );
    const [seven, thirty] = screen.getAllByRole("radio");
    expect(seven).toHaveAttribute("aria-checked", "true");
    expect(seven).toHaveClass("bg-surface", "border-border-strong", "text-primary", "h-6.5");
    expect(thirty).toHaveAttribute("aria-checked", "false");
    expect(thirty).toHaveClass("bg-transparent", "border-transparent", "text-secondary");
  });

  it("selects on click", () => {
    const onChange = vi.fn();
    render(() => <Ranges onChange={onChange} />);
    fireEvent.click(screen.getByRole("radio", { name: "90 days" }));
    expect(onChange).toHaveBeenCalledWith("90");
    expect(screen.getByRole("radio", { name: "90 days" })).toHaveAttribute("aria-checked", "true");
  });

  it("moves with the arrow keys, wraps, and skips disabled options", () => {
    const onChange = vi.fn();
    render(() => <Ranges onChange={onChange} disabled />);
    const [seven, , ninety] = screen.getAllByRole("radio");
    fireEvent.keyDown(seven as HTMLElement, { key: "ArrowRight" });
    expect(onChange).toHaveBeenLastCalledWith("90");
    expect(ninety).toHaveFocus();
    fireEvent.keyDown(ninety as HTMLElement, { key: "ArrowDown" });
    expect(onChange).toHaveBeenLastCalledWith("7");
    fireEvent.keyDown(seven as HTMLElement, { key: "ArrowLeft" });
    expect(onChange).toHaveBeenLastCalledWith("90");
    fireEvent.keyDown(ninety as HTMLElement, { key: "Enter" });
    expect(onChange).toHaveBeenCalledTimes(3);
  });

  it("can be a tablist of tabs with icons, compact, filled, and sized", () => {
    render(() => (
      <SegmentedControl
        kind="tabs"
        label="Views"
        size={40}
        compact
        fill
        unselectedTone="primary"
        segmentClass="px-3!"
        value="board"
        onValueChange={() => {}}
        options={[
          { value: "board", label: "Board", icon: "square-kanban", title: "Board (⌘ 3)" },
          { value: "list", label: "List", icon: "list" },
        ]}
      />
    ));
    expect(screen.getByRole("tablist", { name: "Views" })).toBeInTheDocument();
    const [board, list] = screen.getAllByRole("tab");
    expect(board).toHaveAttribute("aria-selected", "true");
    expect(board).toHaveAttribute("title", "Board (⌘ 3)");
    expect(board).toHaveAttribute("data-compact", "1");
    expect(board).toHaveClass("h-10", "flex-1", "justify-center", "px-3!");
    expect(board?.querySelector("svg")).toHaveAttribute("width", "14");
    expect(list).toHaveAttribute("aria-selected", "false");
    expect(list).toHaveClass("text-primary");
    expect(list).not.toHaveAttribute("aria-checked");
  });
});
