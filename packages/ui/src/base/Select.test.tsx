import { fireEvent, render, screen } from "@solidjs/testing-library";
import { Select } from "./Select";

describe("Select", () => {
  it("renders options from strings and objects and marks the value selected", () => {
    render(() => (
      <Select
        aria-label="Swimlanes"
        value="role"
        options={[{ value: "none", label: "None" }, { value: "role", label: "Role" }, "Package"]}
      />
    ));
    const select = screen.getByRole("combobox", { name: "Swimlanes" }) as HTMLSelectElement;
    expect(select).toHaveClass(
      "h-8",
      "px-2",
      "rounded-sm",
      "border-border-strong",
      "bg-surface",
      "text-primary",
    );
    const options = screen.getAllByRole("option") as HTMLOptionElement[];
    expect(options.map((o) => o.textContent)).toEqual(["None", "Role", "Package"]);
    expect(options.map((o) => o.value)).toEqual(["none", "role", "Package"]);
    expect(options[1]?.selected).toBe(true);
    expect(select.value).toBe("role");
  });

  it("passes the native change event through", () => {
    const onChange = vi.fn(
      (event: Event & { currentTarget: HTMLSelectElement }) => event.currentTarget.value,
    );
    render(() => (
      <Select
        aria-label="Agent"
        options={["Codex", "Claude Code"]}
        value="Codex"
        onChange={onChange}
      />
    ));
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    select.value = "Claude Code";
    fireEvent.change(select);
    expect(onChange).toHaveReturnedWith("Claude Code");
  });

  it("marks a risky choice in red, bold, and disables options", () => {
    render(() => (
      <Select
        aria-label="Permission mode"
        danger
        options={[{ value: "Ask", disabled: true }, "Bypass permissions"]}
        value="Bypass permissions"
      />
    ));
    expect(screen.getByRole("combobox")).toHaveClass(
      "border-status-danger-solid",
      "text-status-danger-text",
      "font-semibold",
    );
    expect(screen.getByRole("option", { name: "Ask" })).toBeDisabled();
  });
});
