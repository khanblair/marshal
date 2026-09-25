import { render } from "@solidjs/testing-library";
import { CiStatus, ciAppearance } from "./CiStatus";

describe("CiStatus", () => {
  it.each([
    ["queued", "spinner", "text-secondary"],
    ["running", "spinner", "text-secondary"],
    ["passed", "check", "text-status-working-text"],
    ["failed", "x", "text-status-danger-text"],
    ["cancelled", "slash", "text-muted"],
    ["unknown", "spinner", "text-secondary"],
  ])("draws %s with %s in %s", (status, icon, color) => {
    expect(ciAppearance(status)).toEqual({ icon, color });
  });

  it("shows the icon and label in the state color, bold", () => {
    const { container } = render(() => <CiStatus status="failed">Failed</CiStatus>);
    const status = container.firstElementChild as HTMLElement;
    expect(status).toHaveTextContent("Failed");
    expect(status).toHaveClass("inline-flex", "gap-1", "font-semibold", "text-status-danger-text");
    expect(status.querySelector("svg")).toHaveAttribute("width", "14");
  });

  it("draws the spinner while running, alone when there is no label", () => {
    const { container } = render(() => (
      <CiStatus status="running" iconSize={12} title="CI running" />
    ));
    const status = container.firstElementChild as HTMLElement;
    expect(status).toHaveAttribute("title", "CI running");
    expect(status.querySelector("svg")).toBeNull();
    expect(status.querySelector(".animate-spin-fast")).not.toBeNull();
    expect(status).toHaveTextContent("");
  });
});
