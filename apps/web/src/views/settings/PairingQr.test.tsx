import { cleanup, render, screen } from "@solidjs/testing-library";
import { afterEach, describe, expect, it } from "vitest";
import { PairingQr } from "./PairingQr";

afterEach(cleanup);

describe("PairingQr", () => {
  it("draws the text as a labelled square QR code, dark on white", () => {
    render(() => (
      <PairingQr text="marshal://pair?host=a%3A1&code=7QX-2LD" label="Pairing QR code" />
    ));
    const svg = screen.getByRole("img", { name: "Pairing QR code" });
    const [, , width, height] = (svg.getAttribute("viewBox") ?? "").split(" ");
    expect(width).toBe(height);
    expect(Number(width)).toBeGreaterThan(20);
    expect(svg.querySelector("rect")).toHaveAttribute("fill", "#fff");
    expect(svg.querySelector("path")?.getAttribute("d")).toMatch(/^M\d+ \d+h1v1h-1z/);
  });

  it("draws a different code for different text", () => {
    const { unmount } = render(() => <PairingQr text="one" label="a" />);
    const first = screen.getByRole("img").querySelector("path")?.getAttribute("d");
    unmount();
    render(() => <PairingQr text="two" label="a" />);
    expect(screen.getByRole("img").querySelector("path")?.getAttribute("d")).not.toBe(first);
  });
});
