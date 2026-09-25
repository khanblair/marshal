import { render } from "@solidjs/testing-library";
import { Icon } from "./Icon";
import { StatusIcon } from "./StatusIcon";
import { isStatusKey, statusIcon, statusTone } from "./status";

describe("StatusIcon", () => {
  it("draws the state's flag in its solid color", () => {
    const { container } = render(() => <StatusIcon state="working" size={14} />);
    const span = container.firstElementChild as HTMLElement;
    expect(span).toHaveClass("text-status-working-solid");
    expect(span.style.width).toBe("14px");
    const flag = render(() => <Icon name="st-working" size={14} />);
    expect(span.innerHTML).toBe((flag.container.firstElementChild as HTMLElement).innerHTML);
  });

  it("colors backlog muted on cards and with the strong border in columns", () => {
    const card = render(() => <StatusIcon state="backlog" />);
    expect(card.container.firstElementChild).toHaveClass("text-muted");
    const column = render(() => <StatusIcon state="backlog" palette="column" />);
    expect(column.container.firstElementChild).toHaveClass("text-border-strong");
  });

  it("shows merging with the ready flag and color", () => {
    const { container } = render(() => <StatusIcon state="merging" class="ml-1" />);
    expect(container.firstElementChild).toHaveClass("text-status-ready-solid", "ml-1");
  });
});

describe("status tables", () => {
  it("maps states to tones and icons like STATUS in store.js", () => {
    expect(statusTone("needs")).toBe("needs-you");
    expect(statusTone("merging")).toBe("ready");
    expect(statusTone("backlog")).toBe("neutral");
    expect(statusTone("danger")).toBe("danger");
    expect(statusTone("constructor")).toBe("neutral");
    expect(statusIcon("review")).toBe("st-review");
    expect(statusIcon("merging")).toBe("st-ready");
    expect(statusIcon("unknown")).toBe("st-backlog");
    expect(isStatusKey("done")).toBe(true);
    expect(isStatusKey("hasOwnProperty")).toBe(false);
  });
});
