import { fireEvent, render, screen } from "@solidjs/testing-library";
import { SettingsPanel } from "./SettingsPanel";

describe("SettingsPanel", () => {
  it("is a bordered surface box that adds layout classes", () => {
    const { container } = render(() => (
      <SettingsPanel class="p-4" data-x="1">
        content
      </SettingsPanel>
    ));
    const panel = container.firstElementChild as HTMLElement;
    expect(panel.tagName).toBe("DIV");
    expect(panel).toHaveClass("border", "border-border", "rounded-lg", "bg-surface", "p-4");
    expect(panel).not.toHaveClass("overflow-hidden");
    expect(panel).toHaveAttribute("data-x", "1");
    expect(panel).toHaveTextContent("content");
  });

  it("stacks and clips rows as a list", () => {
    render(() => (
      <SettingsPanel list role="listbox" aria-label="Roles">
        <div>row</div>
      </SettingsPanel>
    ));
    expect(screen.getByRole("listbox", { name: "Roles" })).toHaveClass(
      "flex",
      "flex-col",
      "overflow-hidden",
    );
  });

  it("is a form when it has onSubmit", () => {
    const onSubmit = vi.fn((event: SubmitEvent) => event.preventDefault());
    const { container } = render(() => (
      <SettingsPanel onSubmit={onSubmit}>
        <button type="submit">Save role</button>
      </SettingsPanel>
    ));
    expect(container.firstElementChild?.tagName).toBe("FORM");
    fireEvent.click(screen.getByRole("button", { name: "Save role" }));
    expect(onSubmit).toHaveBeenCalledOnce();
  });
});
