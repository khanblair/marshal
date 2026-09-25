import { render, screen } from "@solidjs/testing-library";
import { PhoneList } from "./PhoneList";

describe("PhoneList", () => {
  it("is a bare list with 16 px sides and 24 px below", () => {
    render(() => (
      <PhoneList aria-label="Sessions">
        <li>One</li>
      </PhoneList>
    ));
    const list = screen.getByRole("list", { name: "Sessions" });
    expect(list).toHaveClass("m-0", "px-4", "pb-6", "list-none");
    expect(screen.getByRole("listitem")).toHaveTextContent("One");
  });
});
