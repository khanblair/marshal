import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Select } from "./Select";

afterEach(cleanup);

/** Opens the dropdown and returns the options it lists, with their text. */
function openList(trigger: HTMLElement): HTMLElement[] {
  fireEvent.click(trigger);
  return screen.getAllByRole("menuitemradio");
}

describe("Select", () => {
  it("shows the chosen option and lists every option with the chosen one checked", () => {
    render(() => (
      <Select
        aria-label="Swimlanes"
        value="role"
        options={[{ value: "none", label: "None" }, { value: "role", label: "Role" }, "Package"]}
      />
    ));
    const trigger = screen.getByRole("combobox", { name: "Swimlanes" });
    expect(trigger).toHaveClass("h-8", "px-2", "rounded-sm", "border-border-strong", "bg-surface");
    expect(trigger).toHaveTextContent("Role");
    const options = openList(trigger);
    expect(options.map((o) => o.textContent)).toEqual(["None", "Role", "Package"]);
    expect(options.map((o) => o.getAttribute("data-value"))).toEqual(["none", "role", "Package"]);
    expect(options[1]?.getAttribute("aria-checked")).toBe("true");
  });

  it("picks an option, closes the list, and calls onChange with its value", () => {
    const onChange = vi.fn(
      (event: { currentTarget: { value: string } }) => event.currentTarget.value,
    );
    render(() => (
      <Select
        aria-label="Agent"
        name="agent"
        options={["Codex", "Claude Code"]}
        value="Codex"
        onChange={onChange}
      />
    ));
    const trigger = screen.getByRole("combobox", { name: "Agent" });
    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("menuitemradio", { name: "Claude Code" }));
    expect(onChange).toHaveReturnedWith("Claude Code");
    expect(trigger).toHaveTextContent("Claude Code");
    expect(screen.queryByRole("menu")).toBeNull();
    const field = document.querySelector<HTMLInputElement>('input[name="agent"]');
    expect(field?.value).toBe("Claude Code");
  });

  it("does not call onChange when the chosen option is picked again", () => {
    const onChange = vi.fn();
    render(() => (
      <Select
        aria-label="Agent"
        options={["Codex", "Claude Code"]}
        value="Codex"
        onChange={onChange}
      />
    ));
    fireEvent.click(screen.getByRole("combobox", { name: "Agent" }));
    fireEvent.click(screen.getByRole("menuitemradio", { name: "Codex" }));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("closes on Escape and on a press outside, and opens with the arrow key", () => {
    render(() => <Select aria-label="Agent" options={["Codex", "Claude Code"]} value="Codex" />);
    const trigger = screen.getByRole("combobox", { name: "Agent" });
    fireEvent.keyDown(trigger, { key: "ArrowDown" });
    expect(screen.getAllByRole("menuitemradio")).toHaveLength(2);
    fireEvent.keyDown(screen.getAllByRole("menuitemradio")[0] as HTMLElement, { key: "Escape" });
    expect(screen.queryByRole("menu")).toBeNull();
    fireEvent.click(trigger);
    fireEvent.pointerDown(document.body);
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("marks a risky choice in red and bold, and disables options that cannot be picked", () => {
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
    const [ask] = openList(screen.getByRole("combobox"));
    expect(ask).toBeDisabled();
  });
});
