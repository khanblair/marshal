import { fireEvent, render, screen } from "@solidjs/testing-library";
import { Button } from "../base/Button";
import { SettingsSection } from "./SettingsSection";

describe("SettingsSection", () => {
  it("is a section with a 22 px heading and a description pulled up", () => {
    const { container } = render(() => (
      <SettingsSection title="Provider keys" description="The built-in agent uses these keys.">
        <div>rows</div>
      </SettingsSection>
    ));
    const section = container.firstElementChild as HTMLElement;
    expect(section.tagName).toBe("SECTION");
    expect(section).toHaveClass("flex", "flex-col", "gap-4");
    expect(screen.getByRole("heading", { level: 2, name: "Provider keys" })).toHaveClass(
      "m-0",
      "text-view-title",
      "leading-7",
      "font-bold",
    );
    expect(screen.getByText("The built-in agent uses these keys.")).toHaveClass(
      "-mt-2",
      "mb-0",
      "text-secondary",
      "max-w-[72ch]",
    );
  });

  it("puts actions on the heading row", () => {
    render(() => <SettingsSection title="Roles" actions={<Button icon="plus">New role</Button>} />);
    const heading = screen.getByRole("heading", { name: "Roles" });
    expect(heading).toHaveClass("flex-1");
    expect(heading.parentElement).toHaveClass("flex", "flex-wrap", "items-center", "gap-3");
    expect(screen.getByRole("button", { name: "New role" })).toBeInTheDocument();
  });

  it("is a form when it has onSubmit", () => {
    const onSubmit = vi.fn((event: SubmitEvent) => event.preventDefault());
    const { container } = render(() => (
      <SettingsSection title="Profile" onSubmit={onSubmit}>
        <button type="submit">Save profile</button>
      </SettingsSection>
    ));
    expect(container.firstElementChild?.tagName).toBe("FORM");
    fireEvent.click(screen.getByRole("button", { name: "Save profile" }));
    expect(onSubmit).toHaveBeenCalledOnce();
  });
});
