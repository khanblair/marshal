import { render, screen } from "@solidjs/testing-library";
import { PhoneRowMeta } from "./PhoneRowMeta";

describe("PhoneRowMeta", () => {
  it("wraps small facts with 12 px between them", () => {
    render(() => (
      <PhoneRowMeta class="text-secondary">
        <span>Worker</span>
        <span>Codex</span>
      </PhoneRowMeta>
    ));
    const line = screen.getByText("Worker").parentElement;
    expect(line).toHaveClass(
      "flex",
      "flex-wrap",
      "gap-x-3",
      "gap-y-0.5",
      "text-small",
      "text-secondary",
    );
    expect(line).toHaveTextContent("WorkerCodex");
  });
});
