import { fireEvent, render } from "@solidjs/testing-library";
import { Scrim, type ScrimTone } from "./Scrim";

describe("Scrim", () => {
  it("covers its positioned parent with the dialog dim by default and handles clicks", () => {
    const onClick = vi.fn();
    const { container } = render(() => <Scrim class="z-scrim" onClick={onClick} />);
    const scrim = container.firstElementChild as HTMLElement;
    expect(scrim).toHaveAttribute("aria-hidden", "true");
    expect(scrim).toHaveClass("absolute", "inset-0", "bg-scrim-dialog", "z-scrim");
    fireEvent.click(scrim);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it.each([
    ["side", "bg-scrim-side"],
    ["sheet", "bg-scrim-sheet"],
    ["clear", "bg-transparent"],
  ] as [ScrimTone, string][])("draws the %s tone", (tone, cls) => {
    const { container } = render(() => <Scrim tone={tone} fixed />);
    expect(container.firstElementChild).toHaveClass(cls, "fixed");
  });
});
