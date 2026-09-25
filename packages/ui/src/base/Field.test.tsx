import { render, screen } from "@solidjs/testing-library";
import { Field } from "./Field";
import { Input } from "./Input";

describe("Field", () => {
  it("labels its control and shows the hint", () => {
    const { container } = render(() => (
      <Field label="Email" hint="Used only to send briefs by email.">
        <Input type="email" />
      </Field>
    ));
    expect(screen.getByRole("textbox", { name: /^Email/ })).toBeInTheDocument();
    const label = container.firstElementChild as HTMLElement;
    expect(label.tagName).toBe("LABEL");
    expect(label).toHaveClass("flex", "flex-col", "gap-1.5");
    expect(screen.getByText("Email")).toHaveClass("font-medium");
    expect(screen.getByText("Used only to send briefs by email.")).toHaveClass(
      "text-small",
      "leading-4.5",
      "text-secondary",
    );
  });

  it("shows the error instead of the hint", () => {
    render(() => (
      <Field
        label="Name"
        hint="Shown on cards"
        error="Enter a name. It shows on cards you comment on."
      >
        <Input />
      </Field>
    ));
    expect(screen.queryByText("Shown on cards")).toBeNull();
    expect(screen.getByText(/Enter a name/)).toHaveClass("text-small", "text-status-danger-text");
  });

  it("has a compact form with a small secondary label", () => {
    const { container } = render(() => (
      <Field label="Model" compact>
        <select />
      </Field>
    ));
    expect(container.firstElementChild).toHaveClass("gap-1", "min-w-0");
    expect(screen.getByText("Model")).toHaveClass(
      "text-caption",
      "leading-4",
      "text-secondary",
      "font-medium",
    );
  });
});
