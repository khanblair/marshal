import { fireEvent, render, screen } from "@solidjs/testing-library";
import { FOCUS_DELAY_MS } from "./focus-trap";
import { SignIn } from "./SignIn";

const TOKEN = "mt_4f9a1c7e0b2d";

function tokenField(): HTMLInputElement {
  return screen.getByLabelText("Access token") as HTMLInputElement;
}

function type(value: string) {
  fireEvent.input(tokenField(), { target: { value } });
}

describe("SignIn", () => {
  afterEach(() => vi.useRealTimers());

  it("asks for the token and says where to find it", () => {
    const { container } = render(() => <SignIn onSubmit={() => {}} />);
    expect(screen.getByRole("heading", { name: "Sign in to Marshal" })).toBeInTheDocument();
    expect(
      screen.getByText(/Paste the access token for this computer\. To see it, run/),
    ).toHaveTextContent(
      "Paste the access token for this computer. To see it, run marshal token --show in a terminal on the computer where Marshal runs.",
    );
    const code = container.querySelector("code") as HTMLElement;
    expect(code).toHaveTextContent("marshal token --show");
    expect(code).toHaveClass("font-mono", "bg-surface-sunken");
  });

  it("has a password field that the browser does not remember or spell check", () => {
    render(() => <SignIn onSubmit={() => {}} />);
    const field = tokenField();
    expect(field).toHaveAttribute("type", "password");
    expect(field).toHaveAttribute("autocomplete", "off");
    expect(field).toHaveAttribute("spellcheck", "false");
    expect(field).toHaveAttribute("data-autofocus");
    expect(screen.getByRole("form", { name: "Sign in to Marshal" })).toContainElement(field);
  });

  it("focuses the field when it opens, as a dialog does", () => {
    vi.useFakeTimers();
    render(() => <SignIn onSubmit={() => {}} />);
    expect(tokenField()).not.toHaveFocus();
    vi.advanceTimersByTime(FOCUS_DELAY_MS);
    expect(tokenField()).toHaveFocus();
  });

  it("disables Sign in until there is a token that is not blank", () => {
    render(() => <SignIn onSubmit={() => {}} />);
    const button = screen.getByRole("button", { name: "Sign in" });
    expect(button).toBeDisabled();
    type("   ");
    expect(button).toBeDisabled();
    type(TOKEN);
    expect(button).toBeEnabled();
    expect(button).toHaveAttribute("type", "submit");
    expect(button).toHaveClass("bg-ink", "w-full");
  });

  it("submits the trimmed token on the button and on Enter", () => {
    const onSubmit = vi.fn();
    render(() => <SignIn onSubmit={onSubmit} />);
    type(`  ${TOKEN}\n `);
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(onSubmit).toHaveBeenLastCalledWith(TOKEN);
    // Enter in the field is the browser's own form submission, which raises this event.
    fireEvent.submit(screen.getByRole("form"));
    expect(onSubmit).toHaveBeenCalledTimes(2);
    expect(onSubmit).toHaveBeenLastCalledWith(TOKEN);
  });

  it("refuses an empty token even when the form is submitted directly", () => {
    const onSubmit = vi.fn();
    render(() => <SignIn onSubmit={onSubmit} />);
    fireEvent.submit(screen.getByRole("form"));
    type("  ");
    fireEvent.submit(screen.getByRole("form"));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("does nothing while busy, and shows a spinner in the button", () => {
    const onSubmit = vi.fn();
    render(() => <SignIn onSubmit={onSubmit} busy />);
    type(TOKEN);
    const button = screen.getByRole("button", { name: "Sign in" });
    expect(button).toBeDisabled();
    expect(button.querySelector("svg")).toBeNull();
    expect(button.querySelector(".animate-spin-fast")).not.toBeNull();
    expect(screen.getByRole("form")).toHaveAttribute("aria-busy", "true");
    expect(tokenField()).toHaveAttribute("readonly");
    fireEvent.submit(screen.getByRole("form"));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("is not busy by default", () => {
    render(() => <SignIn onSubmit={() => {}} />);
    expect(screen.getByRole("form")).not.toHaveAttribute("aria-busy");
    expect(tokenField()).not.toHaveAttribute("readonly");
  });

  it("shows the caller's error under the field, announced, and the field points at it", () => {
    render(() => (
      <SignIn onSubmit={() => {}} error="That token is not right. Check it and paste it again." />
    ));
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("That token is not right. Check it and paste it again.");
    expect(alert).toHaveClass("text-status-danger-text");
    expect(alert.id).not.toBe("");
    expect(tokenField()).toHaveAttribute("aria-describedby", alert.id);
    expect(tokenField()).toHaveAttribute("aria-invalid", "true");
    expect(tokenField()).toHaveClass("border-status-danger-solid");
    expect(screen.getByLabelText("Access token")).toBe(tokenField());
  });

  it("has no alert and no description when there is no error", () => {
    render(() => <SignIn onSubmit={() => {}} />);
    expect(screen.queryByRole("alert")).toBeNull();
    expect(tokenField()).not.toHaveAttribute("aria-describedby");
    expect(tokenField()).not.toHaveAttribute("aria-invalid");
  });

  it("keeps the token only in the field's own value", () => {
    const logs = (["log", "info", "warn", "error", "debug"] as const).map((method) =>
      vi.spyOn(console, method).mockImplementation(() => {}),
    );
    const { container } = render(() => (
      <SignIn onSubmit={() => {}} error="That token is not right." />
    ));
    type(TOKEN);
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(tokenField().value).toBe(TOKEN);
    expect(container.innerHTML).not.toContain(TOKEN);
    expect(document.body.textContent).not.toContain(TOKEN);
    for (const element of container.querySelectorAll("*")) {
      for (const attribute of element.getAttributeNames()) {
        expect(element.getAttribute(attribute)).not.toContain(TOKEN);
      }
    }
    for (const log of logs) {
      expect(log).not.toHaveBeenCalled();
      log.mockRestore();
    }
  });

  it("uses the phone layout, and fills its container", () => {
    const { container, unmount } = render(() => <SignIn onSubmit={() => {}} />);
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("flex-1", "w-full", "h-full", "overflow-auto", "p-6");
    unmount();
    const phone = render(() => <SignIn onSubmit={() => {}} phone />);
    expect(phone.container.firstElementChild).toHaveClass("px-4", "pt-4");
    expect(phone.container.firstElementChild).not.toHaveClass("p-6");
  });

  it("adds class and passes other attributes to its root", () => {
    const { container } = render(() => (
      <SignIn onSubmit={() => {}} class="bg-canvas" data-testid="sign-in" />
    ));
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("bg-canvas");
    expect(root).toHaveAttribute("data-testid", "sign-in");
  });
});
