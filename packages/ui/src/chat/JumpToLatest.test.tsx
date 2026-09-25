import { fireEvent, render, screen } from "@solidjs/testing-library";
import { JumpToLatest } from "./JumpToLatest";

describe("JumpToLatest", () => {
  it("floats a raised pill from a zero-height row", () => {
    const onClick = vi.fn();
    const { container } = render(() => <JumpToLatest onClick={onClick} />);
    expect(container.firstElementChild).toHaveClass("relative", "h-0");
    const pill = screen.getByRole("button", { name: "Jump to latest" });
    expect(pill).toHaveClass(
      "absolute",
      "left-1/2",
      "-translate-x-1/2",
      "bottom-2.5",
      "rounded-full!",
      "shadow-e1",
      "h-8",
    );
    expect(pill.querySelector("svg")).toHaveAttribute("width", "14");
    fireEvent.click(pill);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("sits 12 px up in chats and takes a label", () => {
    render(() => <JumpToLatest offset={12} label="Newest" />);
    expect(screen.getByRole("button", { name: "Newest" })).toHaveClass("bottom-3");
  });
});
