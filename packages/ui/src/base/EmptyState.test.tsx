import { render, screen } from "@solidjs/testing-library";
import { Button } from "./Button";
import { EmptyState } from "./EmptyState";

describe("EmptyState", () => {
  it("centers an icon, the message, and an action", () => {
    const { container } = render(() => (
      <EmptyState
        icon="square-kanban"
        messageClass="max-w-[40ch]"
        action={<Button variant="primary">New card</Button>}
      >
        This board has no cards yet.
      </EmptyState>
    ));
    expect(container.firstElementChild).toHaveClass(
      "flex-1",
      "items-center",
      "justify-center",
      "gap-3",
      "p-6",
      "text-center",
    );
    expect(container.querySelector("svg")).toHaveAttribute("width", "20");
    const message = screen.getByText("This board has no cards yet.");
    expect(message.tagName).toBe("P");
    expect(message).toHaveClass("m-0", "text-secondary", "max-w-[40ch]");
    expect(screen.getByRole("button", { name: "New card" })).toBeInTheDocument();
  });

  it("works without an icon or action", () => {
    const { container } = render(() => <EmptyState>Nothing here</EmptyState>);
    expect(container.querySelector("svg")).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
