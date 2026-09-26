import { render } from "@solidjs/testing-library";
import { Avatar } from "./Avatar";

const first = (container: HTMLElement) => container.firstElementChild as HTMLElement;

describe("Avatar", () => {
  it("draws a person as a 22 px circle with initials", () => {
    const { container } = render(() => <Avatar initials="AO" title="Ada Okafor" />);
    const avatar = first(container);
    expect(avatar).toHaveTextContent("AO");
    expect(avatar).toHaveAttribute("title", "Ada Okafor");
    expect(avatar).toHaveClass(
      "size-5.5",
      "rounded-full",
      "bg-surface-selected",
      "text-micro",
      "font-bold",
    );
    expect(avatar.querySelector("svg")).toBeNull();
  });

  it("draws a person's picture in place of the initials, filling the circle", () => {
    const { container } = render(() => (
      <Avatar initials="AO" src="blob:http://localhost/abc" size={64} bordered />
    ));
    const avatar = first(container);
    const image = avatar.querySelector("img");
    expect(image).toHaveAttribute("src", "blob:http://localhost/abc");
    expect(image).toHaveAttribute("alt", "");
    expect(image).toHaveClass("size-full", "rounded-full", "object-cover");
    expect(avatar).not.toHaveTextContent("AO");
    expect(avatar).toHaveClass("size-16", "rounded-full", "border");
  });

  it("draws the initials again when there is no picture", () => {
    const { container } = render(() => <Avatar initials="AO" />);
    expect(first(container)).toHaveTextContent("AO");
    expect(first(container).querySelector("img")).toBeNull();
  });

  it("ignores a picture for the agent", () => {
    const { container } = render(() => <Avatar kind="agent" src="blob:x" />);
    expect(first(container).querySelector("img")).toBeNull();
    expect(first(container).querySelector("svg")).not.toBeNull();
  });

  it("draws the agent as a rounded square with a bot icon", () => {
    const { container } = render(() => <Avatar kind="agent" initials="ignored" />);
    const avatar = first(container);
    expect(avatar).toHaveClass(
      "size-5.5",
      "rounded-sm-plus",
      "bg-surface-sunken",
      "text-secondary",
    );
    expect(avatar).not.toHaveTextContent("ignored");
    expect(avatar.querySelector("svg")).toHaveAttribute("width", "12");
  });

  it("rings avatars in a stack, with an outline on the agent", () => {
    const person = render(() => <Avatar initials="BA" ring />);
    expect(first(person.container)).toHaveClass("border-2", "border-surface");
    const agent = render(() => <Avatar kind="agent" ring />);
    expect(first(agent.container)).toHaveClass(
      "border-2",
      "outline-1",
      "outline-border-strong",
      "-outline-offset-2",
    );
  });

  it.each([
    [28, "person", ["size-7", "text-badge", "border", "border-border-strong"]],
    [64, "person", ["size-16", "text-view-title"]],
    [28, "agent", ["size-7", "rounded-md"]],
    [64, "agent", ["size-16", "rounded-lg"]],
  ] as const)("sizes to %i px (%s)", (size, kind, classes) => {
    const { container } = render(() => <Avatar kind={kind} size={size} initials="AO" bordered />);
    expect(first(container)).toHaveClass(...classes);
  });

  it("uses a 14 px bot icon at 28 px", () => {
    const { container } = render(() => <Avatar kind="agent" size={28} />);
    expect(container.querySelector("svg")).toHaveAttribute("width", "14");
  });
});
