import { fireEvent, render, screen } from "@solidjs/testing-library";
import { createSignal } from "solid-js";
import { Composer, type ComposerProps } from "./Composer";

function Harness(props: Partial<ComposerProps> & { onSend: () => void }) {
  const [draft, setDraft] = createSignal("");
  return (
    <Composer
      label="Message"
      placeholder="Message the Orchestrator"
      {...props}
      value={draft()}
      onValueChange={setDraft}
    />
  );
}

describe("Composer", () => {
  it("is a rounded box with a borderless field and a disabled ink send button", () => {
    const { container } = render(() => <Harness onSend={() => {}} />);
    expect(container.firstElementChild).toHaveClass(
      "rounded-lg",
      "border-border-strong",
      "items-end",
      "pl-3",
    );
    const field = screen.getByRole("textbox", { name: "Message" });
    expect(field).toHaveAttribute("placeholder", "Message the Orchestrator");
    expect(field).toHaveAttribute("rows", "1");
    expect(field).toHaveClass("border-none", "outline-none", "resize-none", "max-h-40", "min-h-7");
    const send = screen.getByRole("button", { name: "Send message" });
    expect(send).toBeDisabled();
    expect(send).toHaveClass("size-8", "bg-ink");
  });

  it("sends on Enter and on the button, not on Shift+Enter or when blank", () => {
    const onSend = vi.fn();
    render(() => <Harness onSend={onSend} />);
    const field = screen.getByRole("textbox");
    fireEvent.keyDown(field, { key: "Enter" });
    expect(onSend).not.toHaveBeenCalled();
    fireEvent.input(field, { target: { value: "What is blocked?" } });
    const send = screen.getByRole("button", { name: "Send message" });
    expect(send).toBeEnabled();
    fireEvent.keyDown(field, { key: "Enter", shiftKey: true });
    expect(onSend).not.toHaveBeenCalled();
    fireEvent.keyDown(field, { key: "Enter" });
    expect(onSend).toHaveBeenCalledTimes(1);
    fireEvent.keyDown(field, { key: "a" });
    fireEvent.click(send);
    expect(onSend).toHaveBeenCalledTimes(2);
  });

  it("takes the card panel sizes and exposes the textarea", () => {
    let area: HTMLTextAreaElement | undefined;
    render(() => (
      <Harness
        onSend={() => {}}
        sendSize={28}
        maxHeight={140}
        textareaRef={(el) => {
          area = el;
        }}
      />
    ));
    expect(screen.getByRole("button")).toHaveClass("size-7");
    expect(area).toBe(screen.getByRole("textbox"));
    expect(area).toHaveClass("max-h-35");
  });
});
