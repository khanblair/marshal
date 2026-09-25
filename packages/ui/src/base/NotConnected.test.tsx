import { fireEvent, render, screen } from "@solidjs/testing-library";
import { NotConnected } from "./NotConnected";

describe("NotConnected", () => {
  it("says the service is not connected and offers to connect it", () => {
    const onConnect = vi.fn();
    render(() => <NotConnected service="GitHub" onConnect={onConnect} />);
    expect(screen.getByText("GitHub is not connected.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Connect GitHub" }));
    expect(onConnect).toHaveBeenCalledOnce();
  });

  it("follows the message with what connecting gives you", () => {
    render(() => (
      <NotConnected
        service="GitHub"
        reason="Connect it to open pull requests from cards."
        onConnect={() => {}}
      />
    ));
    expect(
      screen.getByText("GitHub is not connected. Connect it to open pull requests from cards."),
    ).toBeInTheDocument();
  });

  it("has no reason line when none is given", () => {
    render(() => <NotConnected service="Slack" onConnect={() => {}} />);
    expect(screen.getByText("Slack is not connected.").textContent).toBe("Slack is not connected.");
  });

  it("can name the button", () => {
    render(() => <NotConnected service="GitHub" connectLabel="Add GitHub" onConnect={() => {}} />);
    expect(screen.getByRole("button", { name: "Add GitHub" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect GitHub" })).toBeNull();
  });

  it("shows no sample rows or numbers", () => {
    const { container } = render(() => (
      <NotConnected
        service="GitHub"
        reason="Connect it to see pull requests."
        onConnect={() => {}}
      />
    ));
    expect(container.querySelector("li, tr, table, ul, ol, svg + svg")).toBeNull();
    expect(container.textContent).not.toMatch(/\d/);
  });

  it("is an empty state in the free space, with a 20 px plug icon", () => {
    const { container } = render(() => <NotConnected service="GitHub" onConnect={() => {}} />);
    expect(container.firstElementChild).toHaveClass(
      "flex-1",
      "items-center",
      "justify-center",
      "text-center",
    );
    expect(container.querySelector("svg")).toHaveAttribute("width", "20");
  });

  it("adds class and passes other attributes to its root", () => {
    const { container } = render(() => (
      <NotConnected service="GitHub" onConnect={() => {}} class="min-h-40" data-testid="nc" />
    ));
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("min-h-40", "flex-1");
    expect(root).toHaveAttribute("data-testid", "nc");
  });
});
