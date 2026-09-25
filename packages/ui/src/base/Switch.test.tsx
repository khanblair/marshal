import { fireEvent, render, screen } from "@solidjs/testing-library";
import { createSignal } from "solid-js";
import { Switch } from "./Switch";

describe("Switch", () => {
  it("toggles with role switch and ink when on", () => {
    const onCheckedChange = vi.fn();
    function Harness() {
      const [on, setOn] = createSignal(true);
      return (
        <Switch
          label="Turn off Morning brief"
          checked={on()}
          onCheckedChange={(next) => {
            setOn(next);
            onCheckedChange(next);
          }}
        />
      );
    }
    render(() => <Harness />);
    const toggle = screen.getByRole("switch", { name: "Turn off Morning brief" });
    expect(toggle).toHaveAttribute("aria-checked", "true");
    expect(toggle).toHaveClass("w-9", "h-5", "rounded-full", "justify-end", "bg-ink");
    expect(toggle.firstElementChild).toHaveClass("bg-on-ink");
    fireEvent.click(toggle);
    expect(onCheckedChange).toHaveBeenCalledWith(false);
    expect(toggle).toHaveAttribute("aria-checked", "false");
    expect(toggle).toHaveClass("justify-start", "bg-border-strong");
    expect(toggle.firstElementChild).toHaveClass("bg-surface");
  });
});
